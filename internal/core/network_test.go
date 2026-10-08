package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(req *http.Request, status int, text string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(text)), Request: req}
}
func registryFor(t *testing.T, transport roundTripFunc) *Registry {
	t.Helper()
	return &Registry{client: &http.Client{Transport: transport}, loaded: true, endpoints: map[string][]string{"test": {"https://registry.test/rdap"}}, tlds: []string{"test"}, gates: map[string]chan struct{}{}}
}
func TestRegistryRequiresAuthoritativeEvidence(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		body, state string
	}{
		{"registered", 200, `{"objectClassName":"domain","ldhName":"EXAMPLE.TEST"}`, "registered"},
		{"unregistered", 404, `{"errorCode":404,"title":"Not Found"}`, "unregistered"},
		{"invalid404", 404, `{"message":"missing route"}`, "unknown"},
		{"html404", 404, `<html>not found</html>`, "unknown"},
		{"rateLimited", 429, `{"errorCode":429}`, "unknown"},
		{"mismatch", 200, `{"objectClassName":"domain","ldhName":"OTHER.TEST"}`, "unknown"},
		{"wrongClass", 200, `{"objectClassName":"entity","ldhName":"EXAMPLE.TEST"}`, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := registryFor(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/rdap/domain/example.test" {
					t.Errorf("incorrect lookup path %s", req.URL.Path)
				}
				return response(req, tc.status, tc.body), nil
			})
			result := r.Lookup(context.Background(), "example.test")
			if result.State != tc.state {
				t.Fatalf("got %#v, want %s", result, tc.state)
			}
		})
	}
}
func TestRegistryTimeoutAndRedirected404RemainUnknown(t *testing.T) {
	r := registryFor(t, func(req *http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })
	if got := r.Lookup(context.Background(), "example.test"); got.State != "unknown" {
		t.Fatalf("timeout treated as %s", got.State)
	}
	r = registryFor(t, func(req *http.Request) (*http.Response, error) {
		redirected, _ := http.NewRequest("GET", "https://registrar.test/domain/example.test", nil)
		return response(redirected, 404, `{"errorCode":404}`), nil
	})
	if got := r.Lookup(context.Background(), "example.test"); got.State != "unknown" {
		t.Fatalf("registrar's 404 treated as %s", got.State)
	}
}

func TestEmptyAuthoritativeRDAP404(t *testing.T) {
	for _, contentType := range []string{"application/rdap+json", "text/html"} {
		r := registryFor(t, func(req *http.Request) (*http.Response, error) {
			resp := response(req, 404, "")
			resp.Header.Set("Content-Type", contentType)
			return resp, nil
		})
		got := r.Lookup(context.Background(), "example.test")
		expected := "unknown"
		if contentType == "application/rdap+json" {
			expected = "unregistered"
		}
		if got.State != expected {
			t.Fatalf("%s: %s, expected %s", contentType, got.State, expected)
		}
	}
}
func TestAllTLDMetadataExcludesHeaderAndContainsIDNs(t *testing.T) {
	t.Setenv("TERMBELT_CACHE_DIR", t.TempDir())
	r := NewRegistry(http.DefaultClient)
	if err := r.ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.tlds) < 1000 {
		t.Fatalf("only %d TLDs", len(r.tlds))
	}
	foundIDN := false
	for _, v := range r.tlds {
		if v == "version" || v == "2026100800" {
			t.Fatalf("header leaked into TLD list: %s", v)
		}
		if strings.HasPrefix(v, "xn--") {
			foundIDN = true
		}
	}
	if !foundIDN || r.endpoint("example.com") == "" {
		t.Fatal("missing IDN / .com metadata")
	}
}
func TestDomainsAllIncludesUnsupportedAsUnknown(t *testing.T) {
	r := registryFor(t, func(req *http.Request) (*http.Response, error) { return response(req, 404, `{"errorCode":404}`), nil })
	r.tlds = []string{"test", "unsupported"}
	e := &Engine{RDAP: r}
	res, err := e.Domains(context.Background(), Request{Input: "brand", Options: map[string]string{"all": "true"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data := res.Data.(map[string]any)
	counts := data["counts"].(map[string]int)
	if counts["unregistered"] != 1 || counts["unknown"] != 1 {
		t.Fatalf("counts: %v", counts)
	}
}
func TestDomainInputValidationAndIDN(t *testing.T) {
	for _, s := range []string{"-bad.com", "bad-.com", "bad_name.com", "example.com\r\nquery"} {
		if _, err := domainASCII(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	got, err := domainASCII("café.com")
	if err != nil || got != "xn--caf-dma.com" {
		t.Fatalf("IDN = %q, %v", got, err)
	}
	root, err := registrable("https://www.example.co.uk/path")
	if err != nil || root != "example.co.uk" {
		t.Fatalf("registrable = %q, %v", root, err)
	}
}
func TestPrivateIPNeverCallsProvider(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("private address sent to provider")
		return nil, nil
	})
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.1", "::1", "fe80::1", "::ffff:127.0.0.1"} {
		data, err := e.lookupIP(context.Background(), ip)
		if err != nil {
			t.Fatal(err)
		}
		if data.Source != "local classification" {
			t.Fatalf("%s: %#v", ip, data)
		}
	}
}
func TestCustomIPAPIContractAndValidation(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20, IPAPIURL: "https://custom.test"})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/ip/8.8.8.8" {
			t.Errorf("path %s", req.URL.Path)
		}
		return response(req, 200, `{"ip":"8.8.8.8","city":"Test city","country":"Test","asn":"AS15169","org":"Google"}`), nil
	})
	d, err := e.lookupIP(context.Background(), "8.8.8.8")
	if err != nil || d.Organization != "Google" || d.ASN != "AS15169" {
		t.Fatalf("%#v, %v", d, err)
	}
	for _, v := range []string{"ftp://x.test", "https://user:pass@x.test", "x.test", "https://x.test?secret=1", "https://x.test?"} {
		if _, err := ValidateIPAPI(v); err == nil {
			t.Errorf("accepted %s", v)
		}
	}
}
func TestHTTPSFallbackMapsProviderFields(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" {
			t.Fatal("insecure provider")
		}
		if req.URL.Host == "ipwho.is" {
			return response(req, 429, `{"success":false}`), nil
		}
		return response(req, 200, `{"ipAddress":"8.8.8.8","cityName":"Mountain View","countryName":"United States","asn":15169,"asnOrganization":"Google","timeZones":["America/Los_Angeles"]}`), nil
	})
	d, err := e.lookupIP(context.Background(), "8.8.8.8")
	if err != nil || d.City != "Mountain View" || d.ASN != "AS15169" {
		t.Fatalf("fallback %#v, %v", d, err)
	}
}
func TestHTTPInspectsRedirectsAndCapsBody(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/start" {
			r := response(req, 302, "")
			r.Header.Set("Location", "/final")
			return r, nil
		}
		r := response(req, 200, strings.Repeat("a", (1<<20)+100))
		r.Header.Set("Server", "fixture")
		return r, nil
	})
	d, err := e.inspectHTTP(context.Background(), "https://web.test/start")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Truncated || len(d.Body) != 1<<20 || len(d.Redirects) != 1 || d.Status != 200 {
		t.Fatalf("HTTP data = %#v", d)
	}
	if d.Timings["total"] < 0 {
		t.Fatal("negative timing")
	}
}
func TestTLSInspectionDoesNotDisableHTTPTrust(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "hello") }))
	defer server.Close()
	e := NewEngine(Config{TimeoutSeconds: 3})
	result, err := e.TLS(context.Background(), Request{Input: strings.TrimPrefix(server.URL, "https://")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Data.(map[string]any)["trusted"] != false || len(result.Notes) == 0 {
		t.Fatalf("untrusted fixture reported trusted: %#v", result)
	}
	if _, err = e.inspectHTTP(context.Background(), server.URL); err == nil {
		t.Fatal("HTTP accepted an untrusted certificate")
	}
}
func TestNetworkCancellationStopsProvider(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.lookupIP(ctx, "8.8.8.8")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
}
func TestTransferRespectsAggregateByteBudget(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		r := response(req, 200, "")
		r.Body = io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, 1<<20)))
		return r, nil
	})
	n, seconds, err := e.transfer(context.Background(), []string{"https://speed.test/data"}, false, time.Second, 1<<20, nil)
	if err != nil || n != 1<<20 || seconds <= 0 {
		t.Fatalf("transfer: %d, %f, %v", n, seconds, err)
	}
}
func TestFastParserMatchesPublicScript(t *testing.T) {
	html := []byte(`<script src="/app-a1b2.js"></script>`)
	if len(fastScriptPattern.FindSubmatch(html)) != 2 {
		t.Fatal("script parse failed")
	}
	js := []byte(`getTestOcasParams:{https:!0,token:"YXNkZmFzZGxmbnNkYWZoYXNkZmhrYWxm",urlCount:5}`)
	if len(fastTokenPattern.FindSubmatch(js)) != 2 {
		t.Fatal("token parse failed")
	}
}
func TestConfigRoundtripDoesNotStoreToolInputs(t *testing.T) {
	t.Setenv("TERMBELT_CONFIG", t.TempDir()+"/config.json")
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Set("ip-api-url", "https://api.test/"); err != nil {
		t.Fatal(err)
	}
	c2, err := LoadConfig()
	if err != nil || c2.IPAPIURL != "https://api.test" {
		t.Fatalf("config: %#v, %v", c2, err)
	}
	b, _ := json.Marshal(c2)
	if strings.Contains(string(b), "password") || strings.Contains(string(b), "token") {
		t.Fatal("private tool data persisted")
	}
}

func TestWHOISIncludesOnlyMatchingRegistrarRecord(t *testing.T) {
	for _, name := range []string{"example.com", "wrong.com"} {
		t.Run(name, func(t *testing.T) {
			e := NewEngine(Config{TimeoutSeconds: 20})
			transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "registrar.test" {
					return response(req, 200, fmt.Sprintf(`{"objectClassName":"domain","ldhName":%q,"entities":[{"roles":["registrant"],"vcardArray":["vcard",[["fn",{},"text","Public fixture"]]]}]}`, name)), nil
				}
				return response(req, 200, `{"objectClassName":"domain","ldhName":"example.com","links":[{"rel":"related","href":"https://registrar.test/rdap/domain/example.com"}]}`), nil
			})
			e.Client.Transport = transport
			e.RDAP = registryFor(t, transport)
			e.RDAP.endpoints = map[string][]string{"com": {"https://registry.test/rdap"}}
			result, err := e.Whois(context.Background(), Request{Input: "example.com"})
			if err != nil {
				t.Fatal(err)
			}
			_, included := result.Data.(map[string]any)["registrar_record"]
			if included != (name == "example.com") {
				t.Fatalf("registrar match validation failed: %v", included)
			}
		})
	}
}
func TestPrivateHostingSuffixIsNotRegistrationBoundary(t *testing.T) {
	root, err := registrable("example.github.io")
	if err != nil || root != "github.io" {
		t.Fatalf("got %q, %v", root, err)
	}
}
