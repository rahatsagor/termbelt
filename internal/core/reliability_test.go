package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRegistryRateLimitSuppressesFurtherRequests(t *testing.T) {
	var calls atomic.Int64
	r := registryFor(t, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		resp := response(req, 429, `<html>Too Many Requests</html>`)
		resp.Header.Set("Retry-After", "60")
		return resp, nil
	})
	for _, domain := range []string{"first.test", "second.test", "third.test"} {
		got := r.Lookup(context.Background(), domain)
		if got.State != "unknown" || !strings.Contains(got.Reason, "retry after") {
			t.Fatalf("rate limited result = %#v", got)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("rate-limited registry received %d calls", calls.Load())
	}
}

func TestRegistryRejectsDowngradedAndLookalikeRDAP404s(t *testing.T) {
	for _, tc := range []struct{ scheme, mediaType string }{
		{"http", "application/rdap+json"},
		{"https", "application/rdap+json-invalid"},
	} {
		r := registryFor(t, func(req *http.Request) (*http.Response, error) {
			final, _ := http.NewRequest("GET", tc.scheme+"://registry.test/rdap/domain/example.test", nil)
			resp := response(final, 404, "")
			resp.Header.Set("Content-Type", tc.mediaType)
			return resp, nil
		})
		if got := r.Lookup(context.Background(), "example.test"); got.State != "unknown" {
			t.Fatalf("%v resulted in %s", tc, got.State)
		}
	}
}

func TestRegistryCorruptCacheFallsBackToBundledMetadata(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("TERMBELT_CACHE_DIR", cache)
	for name, contents := range map[string]string{"rdap.json": `{"services":[]}`, "tlds.txt": "JUNK\n"} {
		if err := os.WriteFile(filepath.Join(cache, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRegistry(http.DefaultClient)
	if err := r.ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.tlds) < 1000 || r.endpoint("example.com") == "" {
		t.Fatal("corrupt cache disabled bundled metadata")
	}
}

func TestRegistryRefreshUsesFreshDataWithUnavailableCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "file-not-directory")
	if err := os.WriteFile(cache, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TERMBELT_CACHE_DIR", cache)
	r := NewRegistry(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, ".json") {
			return response(req, 200, `{"services":[[["com"],["https://fresh.test/rdap"]]]}`), nil
		}
		return response(req, 200, "# Version 2026100800, Last Updated today\nCOM\nNET\n"), nil
	})})
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.endpoint("brand.com") != "https://fresh.test/rdap" || len(r.tlds) != 2 {
		t.Fatalf("refresh did not use fresh data: %v", r.tlds)
	}
}

func TestRegistryInvalidRefreshDoesNotReplaceWorkingMetadata(t *testing.T) {
	t.Setenv("TERMBELT_CACHE_DIR", t.TempDir())
	r := registryFor(t, func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, ".json") {
			return response(req, 200, `{"services":[[["com"],["https://fresh.test"]]]}`), nil
		}
		return response(req, 200, "INVALID\n"), nil
	})
	if err := r.Refresh(context.Background()); err == nil {
		t.Fatal("invalid refresh accepted")
	}
	if r.endpoint("brand.test") != "https://registry.test/rdap" {
		t.Fatal("invalid refresh replaced working metadata")
	}
	if _, err := os.Stat(filepath.Join(cacheDir(), "rdap.json")); !os.IsNotExist(err) {
		t.Fatal("partial refresh was cached")
	}
}

func TestDomainSchedulingInterleavesProvidersAndPreservesResultOrder(t *testing.T) {
	var calls []string
	r := registryFor(t, func(req *http.Request) (*http.Response, error) {
		calls = append(calls, strings.TrimPrefix(req.URL.Path, "/domain/"))
		return response(req, 404, `{"errorCode":404}`), nil
	})
	r.endpoints = map[string][]string{"a": {"https://one.test"}, "b": {"https://one.test"}, "c": {"https://two.test"}}
	e := &Engine{RDAP: r}
	result, err := e.Domains(context.Background(), Request{Input: "brand", Options: map[string]string{"tlds": "a,b,c", "concurrency": "1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(calls) != "[brand.a brand.c brand.b]" {
		t.Fatalf("request order = %v", calls)
	}
	results := result.Data.(map[string]any)["domains"].([]RDAPLookup)
	if results[0].Domain != "brand.a" || results[1].Domain != "brand.b" || results[2].Domain != "brand.c" {
		t.Fatalf("result order changed: %v", results)
	}
}

func TestTransferSmallBudgetStillUsesThreeConnections(t *testing.T) {
	ready := make(chan struct{})
	var calls atomic.Int64
	var hosts sync.Map
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		hosts.Store(req.URL.Host, true)
		if calls.Add(1) == 3 {
			close(ready)
		}
		select {
		case <-ready:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(&repeatedByteReader{remaining: 1 << 20, value: 'x'}), Request: req}, nil
	})
	n, _, err := e.transfer(context.Background(), []string{"https://one.test", "https://two.test", "https://three.test"}, false, time.Second, 1<<20, nil)
	if err != nil || n != 1<<20 {
		t.Fatalf("transfer = %d, %v", n, err)
	}
	count := 0
	hosts.Range(func(_, _ any) bool { count++; return true })
	if count != 3 {
		t.Fatalf("only %d connections received a budget", count)
	}
}

func TestTransferEmptyAndHTMLResponsesFailWithoutLooping(t *testing.T) {
	for _, body := range []string{"", "<html>Error</html>"} {
		var calls atomic.Int64
		e := NewEngine(Config{TimeoutSeconds: 20})
		e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			resp := response(req, 200, body)
			if body != "" {
				resp.Header.Set("Content-Type", "text/html")
			}
			return resp, nil
		})
		n, _, err := e.transfer(context.Background(), []string{"https://speed.test"}, false, time.Second, 1<<20, nil)
		if err == nil || n != 0 || calls.Load() != 3 {
			t.Fatalf("body=%q: bytes=%d calls=%d error=%v", body, n, calls.Load(), err)
		}
	}
}

func TestTransferShortBodiesRefundUnusedDownloadBudget(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(req, 200, strings.Repeat("x", 1024)), nil
	})
	n, _, err := e.transfer(context.Background(), []string{"https://speed.test"}, false, time.Second, 64<<10, nil)
	if err != nil || n != 64<<10 {
		t.Fatalf("short bodies prematurely exhausted budget: %d, %v", n, err)
	}
}

func TestTransferReportsPartialServerFailure(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "bad.test" {
			return response(req, 503, "Unavailable"), nil
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(&repeatedByteReader{remaining: 1 << 20, value: 'x'}), Request: req}, nil
	})
	stats, err := e.transferDetailed(context.Background(), []string{"https://bad.test", "https://good.test"}, false, time.Second, 1<<20, nil)
	if err != nil || stats.bytes == 0 || len(stats.failures) == 0 || stats.stopReason != "server-error" {
		t.Fatalf("partial measurement = %#v, %v", stats, err)
	}
}

func TestTransferUploadAdaptsRequestSizeWithinBudget(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	var sent, calls atomic.Int64
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n, err := io.Copy(io.Discard, req.Body)
		if err != nil {
			return nil, err
		}
		if n != req.ContentLength {
			t.Errorf("sent %d bytes, ContentLength %d", n, req.ContentLength)
		}
		sent.Add(n)
		calls.Add(1)
		return response(req, 204, ""), nil
	})
	n, _, err := e.transfer(context.Background(), []string{"https://speed.test"}, true, time.Second, 4<<20, nil)
	if err != nil || n != 4<<20 || sent.Load() != n {
		t.Fatalf("upload bytes=%d sent=%d err=%v", n, sent.Load(), err)
	}
	if calls.Load() >= 16 {
		t.Fatalf("fixed-size request overhead remains: %d requests", calls.Load())
	}
}

func TestMalformedIPResponseFallsBackAndMappedPrivateIPStaysLocal(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "ipwho.is" {
			return response(req, 200, `{"success":true,"ip":"1.1.1.1"}`), nil
		}
		return response(req, 200, `{"ipAddress":"8.8.8.8"}`), nil
	})
	d, err := e.lookupIP(context.Background(), "8.8.8.8")
	if err != nil || d.IP != "8.8.8.8" || d.Source != "https://free.freeipapi.com" {
		t.Fatalf("mismatched IP was trusted: %#v, %v", d, err)
	}
	e.Client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Error("mapped private address contacted provider")
		return nil, fmt.Errorf("unexpected network")
	})
	d, err = e.lookupIP(context.Background(), "::ffff:192.168.1.1")
	if err != nil || d.IP != "192.168.1.1" || d.Source != "local classification" {
		t.Fatalf("mapped IP = %#v, %v", d, err)
	}
}

func FuzzDomainNormalization(f *testing.F) {
	for _, input := range []string{"example.com", "café.com。", "https://example.com/path", "bad\x1b[31m.test", strings.Repeat("x", 64) + ".com"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}
		name, err := domainASCII(input)
		if err == nil {
			again, err := domainASCII(name)
			if err != nil || name != again || len(name) > 253 {
				t.Fatalf("unstable normalization: %q -> %q (%v)", name, again, err)
			}
		}
	})
}

func FuzzTimestampParsing(f *testing.F) {
	for _, input := range []string{"1735689600123456789", "-0.000000001", "2025-01-01T00:00:00Z", "99999999999999999999999999999999999999999999", "+0"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			t.Skip()
		}
		date, err := parseTimeUnit(input, time.UTC, "auto")
		if err == nil && (date.Year() < 1 || date.Year() > 9999) {
			t.Fatalf("out-of-range date: %v", date)
		}
	})
}
