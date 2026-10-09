package core

import (
	"context"
	"strings"
	"testing"
)

func TestClassifyWhoisResponses(t *testing.T) {
	cases := []struct {
		name, domain, text, want string
	}{
		{"identity digital not found", "free-name.io", "Domain not found.\r\n>>> Last update of WHOIS database: 2026-10-09T07:05:33Z <<<\r\n", "unregistered"},
		{"identity digital registered", "nic.io", "Domain Name: nic.io\r\nRegistry Domain ID: REDACTED\r\nCreation Date: 2003-09-15T11:13:53Z\r\nRegistrar: Registry Operator acts as Registrar (9999)\r\n", "registered"},
		{"denic free", "free-name.de", "Domain: free-name.de\nStatus: free\n", "unregistered"},
		{"denic connected", "nic.de", "Domain: nic.de\nStatus: connect\n", "registered"},
		{"pknic not registered", "free-name.pk", "# WHOIS .PK Domains (PKNIC)\n\n    Domain: free-name.pk\n    Status: Not Registered, and may be available if valid\n    Available: Yes.\n", "unregistered"},
		{"status no object found", "free-name.tc", "Domain Name: free-name.tc\nDomain Status: No Object Found\n", "unregistered"},
		{"registered record with empty contacts", "nic.mq", "domain: nic.mq\nnameserver: ns1-mq.mediaserv.net\n\n[holder]\nNO OBJECT FOUND!\nobject: 2058776466\n", "unknown"},
		{"restricted string", "free-name.bw", "Domain Name: free-name.bw\nDomain Status: Prohibited String - Domain Cannot Be Registered\n", "unknown"},
		{"client refused", "free-name.ch", "Requests of this client are not permitted. Please use https://www.nic.ch/whois/ for queries.\n", "unknown"},
		{"error coded not registered", "free-name.rs", "%ERROR:103: Domain is not registered\n", "unregistered"},
		{"rate limited", "nic.aw", "Error: ratelimit exceeded\n", "unknown"},
		{"empty", "nic.example", "\r\n", "unknown"},
		{"banner without record", "nic.kz", "Whois Server for the .kz top level domain.\nThis server is maintained by KazNIC.\n", "unknown"},
	}
	for _, c := range cases {
		if got, reason := classifyWhois(c.domain, c.text); got != c.want {
			t.Errorf("%s: state = %s (%s), want %s", c.name, got, reason, c.want)
		}
	}
}

func TestDomainsFallBackToWhoisWithoutRDAP(t *testing.T) {
	r := registryFor(t, nil)
	queries := []string{}
	r.queryWhois = func(_ context.Context, server, query string) (string, error) {
		queries = append(queries, server+" "+query)
		if query == "taken.io" {
			return "Domain Name: taken.io\nCreation Date: 2003-09-15T11:13:53Z\nRegistrar: Example Registrar, Inc.\n", nil
		}
		return "Domain not found.\n", nil
	}
	free := r.Lookup(context.Background(), "free.io")
	taken := r.Lookup(context.Background(), "taken.io")
	if free.State != "unregistered" || free.Protocol != "whois" || free.Source != "whois://whois.nic.io" {
		t.Fatalf("free lookup = %#v", free)
	}
	if taken.State != "registered" || taken.Registrar != "Example Registrar, Inc." {
		t.Fatalf("taken lookup = %#v", taken)
	}
	if strings.Join(queries, ",") != "whois.nic.io free.io,whois.nic.io taken.io" {
		t.Fatalf("queries = %v", queries)
	}
	if unsupported := r.Lookup(context.Background(), "name.zz"); unsupported.State != "unknown" || !strings.Contains(unsupported.Reason, "No RDAP or WHOIS") {
		t.Fatalf("unsupported lookup = %#v", unsupported)
	}
}

func TestWhoisTextNormalizesLineEndings(t *testing.T) {
	if got := normalizeWhoisText("a\r\nb\rc\n\n"); got != "a\nb\nc" {
		t.Fatalf("normalized = %q", got)
	}
}
