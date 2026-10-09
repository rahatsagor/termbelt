package core

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"
)

//go:embed iana-whois.json
var embeddedWhois []byte

// bundledWhois maps TLDs that have no RDAP bootstrap entry to the legacy
// WHOIS server IANA publishes for them. Refresh with scripts/whois_servers.py.
var bundledWhois = func() map[string]string {
	var data struct {
		Servers map[string]string `json:"servers"`
	}
	if err := json.Unmarshal(embeddedWhois, &data); err != nil {
		panic("invalid bundled WHOIS metadata: " + err.Error())
	}
	servers := map[string]string{}
	for tld, server := range data.Servers {
		if validateLabel(tld) == nil && validWhoisServer(server) {
			servers[tld] = server
		}
	}
	return servers
}()

func validWhoisServer(server string) bool {
	if !strings.Contains(server, ".") {
		return false
	}
	_, err := domainASCII(server)
	return err == nil
}

func queryWhois(ctx context.Context, server, query string) (string, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(server, "43"))
	if err != nil {
		return "", err
	}
	defer conn.Close()
	deadline := time.Now().Add(8 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if _, err = io.WriteString(conn, query+"\r\n"); err != nil {
		return "", err
	}
	b, err := readLimited(conn, 1<<20)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	return normalizeWhoisText(string(b)), nil
}

func normalizeWhoisText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

// whoisServer returns the legacy WHOIS server for a TLD: the bundled snapshot
// first, then a live IANA referral when allowed.
func (r *Registry) whoisServer(ctx context.Context, tld string, live bool) (string, error) {
	if server := bundledWhois[tld]; server != "" {
		return server, nil
	}
	if !live {
		return "", nil
	}
	root, err := r.whoisQuery(ctx, "whois.iana.org", tld)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(root, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "whois") {
			server := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
			if !validWhoisServer(server) {
				return "", fmt.Errorf("invalid WHOIS referral")
			}
			return server, nil
		}
	}
	return "", nil
}

func (r *Registry) whoisQuery(ctx context.Context, server, query string) (string, error) {
	if r.queryWhois != nil {
		return r.queryWhois(ctx, server, query)
	}
	return queryWhois(ctx, server, query)
}

// Lookups through legacy WHOIS are used only when IANA publishes no RDAP
// service. Free-form responses are classified conservatively: anything that
// is not clearly free or clearly registered stays unknown.
func (r *Registry) lookupWhois(ctx context.Context, domain, server string) RDAPLookup {
	result := RDAPLookup{Domain: domain, State: "unknown", Source: "whois://" + server, Protocol: "whois"}
	release, err := r.acquire(ctx, result.Source)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	defer release()
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	text, err := r.whoisQuery(queryCtx, server, domain)
	if err != nil && queryCtx.Err() == nil {
		// Many WHOIS servers reset bursts of connections; one paced retry
		// usually succeeds without hammering the registry.
		select {
		case <-time.After(time.Second):
			text, err = r.whoisQuery(queryCtx, server, domain)
		case <-queryCtx.Done():
		}
	}
	if err != nil {
		result.Reason = "WHOIS: " + err.Error()
		return result
	}
	result.State, result.Reason = classifyWhois(domain, text)
	if result.State == "registered" {
		result.Registrar = whoisField(text, "registrar", "registrar name", "sponsoring registrar", "registrar organization")
	}
	return result
}

var (
	whoisFree       = regexp.MustCompile(`(?im)(^\s*(domain\s+)?status\s*:\s*(free|available|not\s+registered|no\s+object\s+found)\b|^\s*available\s*:\s*yes\b|registration\s+status\s*:\s*available|\bis\s+free\b|\bis\s+available\s+for\s+(purchase|registration)\b)`)
	whoisNotFound   = regexp.MustCompile(`(?i)(no\s+match|not\s+found|no\s+data\s+found|no\s+entries\s+found|no\s+object\s+found|object\s+does\s+not\s+exist|queried\s+object\s+does\s+not\s+exist|has\s+not\s+been\s+registered|domain\s+is\s+not\s+registered|no\s+matching\s+record|nincs\s+talalat|no\s+record\s+found|was\s+not\s+found|not\s+known\s+in|object_not_found|^\s*no\s+found\s*$)`)
	whoisRestricted = regexp.MustCompile(`(?i)(not\s+available\s+for\s+registration|prohibited\s+string|cannot\s+be\s+registered|can\s+not\s+be\s+registered|reserved\s+domain|usage\s+restriction|restricted\s+word|is\s+not\s+available|registration\s+status\s*:\s*invalid)`)
	whoisRefused    = regexp.MustCompile(`(?i)(rate\s*limit|quota|exceeded|not\s+permitted|access\s+denied|too\s+many|try\s+again|invalid\s+(domain|pattern|query)|wrong\s+top\s+level|only\s+accepts|solo\s+acepta|tld\s+not\s+supported|blacklist)`)
	whoisRegistered = regexp.MustCompile(`(?im)(^\s*(creation\s+date|created(\s+on)?|record\s+created|registered(\s+on)?|registration\s+(date|time)|registrar|sponsoring\s+registrar|name\s*servers?|nserver|expir\w*(\s+date)?|paid-till|registry\s+expiry\s+date|domain\s+status|status|holder|registrant(\s+name)?|domain\s+managers)\s*[:.]|^\s*\[(name\s+server|created\s+on|registrant|status|state)\]|registration\s+status\s*:\s*(busy|active|registered|ok))`)
)

func classifyWhois(domain, text string) (state, reason string) {
	text = normalizeWhoisText(text)
	if text == "" {
		return "unknown", "WHOIS server returned an empty response"
	}
	if whoisRestricted.MatchString(text) {
		return "unknown", "Registry reports the name is reserved, restricted or unavailable"
	}
	evidence := whoisRegistered.MatchString(text) && strings.Contains(strings.ToLower(text), strings.ToLower(domain))
	if whoisFree.MatchString(text) {
		return "unregistered", "Registry WHOIS reports the name as available"
	}
	notFound := whoisNotFound.MatchString(text)
	switch {
	case notFound && !evidence:
		return "unregistered", "Registry WHOIS reports no registration record"
	case notFound && evidence:
		return "unknown", "WHOIS response is ambiguous"
	case evidence:
		return "registered", "Registry WHOIS returned a registration record"
	case whoisRefused.MatchString(text):
		return "unknown", "WHOIS refused the query: " + firstMeaningfulLine(text)
	}
	return "unknown", "Unrecognized WHOIS response"
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "%") && !strings.HasPrefix(line, "#") {
			if len(line) > 120 {
				line = line[:120] + "…"
			}
			return line
		}
	}
	return "no details"
}

func whoisField(text string, keys ...string) string {
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		for _, want := range keys {
			if key == want && value != "" {
				return value
			}
		}
	}
	return ""
}
