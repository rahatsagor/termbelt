package core

import (
	"bufio"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSplitHostPortAcceptsURLsIPv6AndInternationalNames(t *testing.T) {
	for _, tc := range []struct{ input, host, port string }{
		{"https://example.com", "example.com", "443"},
		{"http://example.com", "example.com", "80"},
		{"https://example.com:8443/path?q=1", "example.com", "8443"},
		{"example.com/health", "example.com", "443"},
		{"Example.COM:22", "example.com", "22"},
		{"bücher.de:443", "xn--bcher-kva.de", "443"},
		{"bücher.de", "xn--bcher-kva.de", "443"},
		{"[2001:db8::1]:8443", "2001:db8::1", "8443"},
		{"2001:db8::1", "2001:db8::1", "443"},
		{"192.0.2.1:25", "192.0.2.1", "25"},
	} {
		host, port, err := splitHostPort(tc.input, "443")
		if err != nil || host != tc.host || port != tc.port {
			t.Errorf("%s = %s %s %v; want %s %s", tc.input, host, port, err, tc.host, tc.port)
		}
	}
	for _, input := range []string{"", "example.com:0", "example.com:99999", ":443", "ftp://example.com"} {
		if _, _, err := splitHostPort(input, "443"); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestDNSResolverParsing(t *testing.T) {
	for input, want := range map[string]string{"1.1.1.1": "1.1.1.1:53", "1.1.1.1:5353": "1.1.1.1:5353", "[2606:4700::1111]": "[2606:4700::1111]:53", "2606:4700::1111": "[2606:4700::1111]:53"} {
		if got, err := dnsResolver(input); err != nil || got != want {
			t.Errorf("%s = %s, %v; want %s", input, got, err, want)
		}
	}
	if _, err := dnsResolver("dns.google"); err == nil {
		t.Error("accepted a hostname resolver")
	}
}

func TestNonPublicAddressesStayLocal(t *testing.T) {
	e := NewEngine(Config{TimeoutSeconds: 20})
	for input, kind := range map[string]string{"100.64.1.1": "Carrier-grade NAT shared address", "192.0.2.10": "Reserved or documentation address", "10.0.0.1": "Private network", "127.0.0.1": "Loopback address", "2001:db8::1": "Reserved or documentation address"} {
		d, err := e.lookupIP(t.Context(), input)
		if err != nil || d.Source != "local classification" || d.Organization != kind {
			t.Errorf("%s = %#v, %v", input, d, err)
		}
	}
}

func TestCIDRAcceptsBareAddressesAndReportsHostRange(t *testing.T) {
	single, err := RunLocal(localRequest("cidr", "10.0.0.1", nil))
	if err != nil || single.Data.(map[string]any)["network"] != "10.0.0.1/32" {
		t.Fatalf("bare IPv4 = %#v, %v", single.Data, err)
	}
	v6, err := RunLocal(localRequest("cidr", "2001:db8::1", nil))
	if err != nil || v6.Data.(map[string]any)["network"] != "2001:db8::1/128" {
		t.Fatalf("bare IPv6 = %#v, %v", v6.Data, err)
	}
	subnet, err := RunLocal(localRequest("cidr", "192.168.1.77/24", nil))
	data := subnet.Data.(map[string]any)
	if err != nil || data["first_host"] != "192.168.1.1" || data["last_host"] != "192.168.1.254" || data["wildcard"] != "0.0.0.255" || data["netmask"] != "255.255.255.0" {
		t.Fatalf("subnet = %#v, %v", data, err)
	}
}

func TestJWTReportsLifecycleAndUnsignedTokens(t *testing.T) {
	segment := func(v string) string { return base64.RawURLEncoding.EncodeToString([]byte(v)) }
	future := time.Now().Add(time.Hour).Unix()
	claims := `{"nbf":` + strconv.FormatInt(future, 10) + `,"exp":` + strconv.FormatInt(future+60, 10) + `}`
	result, err := RunLocal(localRequest("jwt", segment(`{"alg":"none"}`)+"."+segment(claims)+".", nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.Data.(map[string]any)["status"] != "not yet valid" || !strings.Contains(strings.Join(result.Notes, " "), "unsigned") {
		t.Fatalf("jwt = %#v %v", result.Data, result.Notes)
	}
}

func TestConfigSetAndResetRepairInvalidSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("TERMBELT_CONFIG", path)
	t.Setenv("TERMBELT_IP_API_URL", "")
	if err := os.WriteFile(path, []byte(`{"timeout_seconds":1,"favorites":["json"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err == nil {
		t.Fatal("strict load accepted invalid timeout")
	}
	c, warning, err := LoadEffectiveConfig()
	if err != nil || warning == nil || c.TimeoutSeconds != 20 {
		t.Fatalf("effective config = %#v, %v, %v", c, warning, err)
	}
	if err := c.Set("timeout-seconds", "30"); err != nil {
		t.Fatal(err)
	}
	repaired, err := LoadConfig()
	if err != nil || repaired.TimeoutSeconds != 30 || strings.Join(repaired.Favorites, ",") != "json" {
		t.Fatalf("repaired = %#v, %v", repaired, err)
	}
	if err := os.WriteFile(path, []byte(`not json`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ResetConfig(); err != nil {
		t.Fatal(err)
	}
	if reset, err := LoadConfig(); err != nil || reset.TimeoutSeconds != 20 {
		t.Fatalf("reset = %#v, %v", reset, err)
	}
	if err := c.Set("favorites", "dns, tls"); err != nil {
		t.Fatal(err)
	}
	if fav, _ := LoadConfig(); strings.Join(fav.Favorites, ",") != "dns,tls" {
		t.Fatalf("favorites = %v", fav.Favorites)
	}
}

func TestListenerParsers(t *testing.T) {
	ss := "State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process\n" +
		"LISTEN 0      4096   127.0.0.53%lo:53      0.0.0.0:*     users:((\"systemd-resolve\",pid=611,fd=14))\n" +
		"LISTEN 0      511    [::]:3000          [::]:*        users:((\"node\",pid=1200,fd=20),(\"node\",pid=1201,fd=20))\n" +
		"LISTEN 0      128    0.0.0.0:22         0.0.0.0:*\n"
	got := parseSS(ss)
	if len(got) != 4 || got[0].Address != "127.0.0.53" || got[0].Port != 53 || got[0].Process != "systemd-resolve" || got[2].PID != 1201 || got[3].Port != 22 || got[3].PID != 0 {
		t.Fatalf("ss = %#v", got)
	}
	proc := "  sl  local_address rem_address   st\n   0: 0100007F:1F90 00000000:0000 0A 00000000\n   1: 0100007F:0050 0100007F:9C40 01 00000000\n"
	listeners := parseProcNet(bufio.NewScanner(strings.NewReader(proc)))
	if len(listeners) != 1 || listeners[0].Address != "127.0.0.1" || listeners[0].Port != 8080 {
		t.Fatalf("proc = %#v", listeners)
	}
	lsof := "p95\ncnode\nf20\nn*:3000\nn127.0.0.1:9229\np7\ncpostgres\nf5\nn[::1]:5432\n"
	parsed := parseLsof(lsof)
	if len(parsed) != 3 || parsed[0].Address != "0.0.0.0" || parsed[2].Address != "::1" || parsed[2].Process != "postgres" || parsed[2].PID != 7 {
		t.Fatalf("lsof = %#v", parsed)
	}
}

func TestLatencySummaryUsesMedianAndJitter(t *testing.T) {
	s := summarizeLatency([]float64{10, 30, 20, 20, 100})
	if s.median != 20 || s.jitter != (20+10+0+80)/4.0 {
		t.Fatalf("summary = %#v", s)
	}
}
