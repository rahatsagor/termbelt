package core

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

//go:embed iana-rdap.json
var embeddedRDAP []byte

//go:embed iana-tlds.txt
var embeddedTLDs string

type bootstrap struct {
	Services [][][]string `json:"services"`
}
type Registry struct {
	client    *http.Client
	mu        sync.Mutex
	loaded    bool
	endpoints map[string][]string
	tlds      []string
	gates     map[string]chan struct{}
	cooldowns map[string]time.Time
	// queryWhois replaces the TCP port 43 client in tests.
	queryWhois func(ctx context.Context, server, query string) (string, error)
}

func NewRegistry(client *http.Client) *Registry {
	return &Registry{client: client, gates: map[string]chan struct{}{}, cooldowns: map[string]time.Time{}}
}
func cacheDir() string {
	if v := os.Getenv("TERMBELT_CACHE_DIR"); v != "" {
		return v
	}
	p, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(p, "termbelt")
}
func cacheMetadata(name string, b []byte) {
	base := cacheDir()
	if base == "" {
		return
	}
	if os.MkdirAll(base, 0700) != nil {
		return
	}
	f, err := os.CreateTemp(base, ".metadata-*")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Close()
		if err == nil {
			_ = os.Rename(f.Name(), filepath.Join(base, name))
		}
	} else {
		f.Close()
	}
}
func loadMetadata(name string) []byte {
	base := cacheDir()
	if base == "" {
		return nil
	}
	p := filepath.Join(base, name)
	info, err := os.Stat(p)
	if err != nil || time.Since(info.ModTime()) > 7*24*time.Hour || info.Size() > maxResponse {
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, _ := readLimited(f, maxResponse)
	return b
}

func parseBootstrap(b []byte) (map[string][]string, error) {
	var data bootstrap
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	endpoints := map[string][]string{}
	for _, service := range data.Services {
		if len(service) != 2 {
			continue
		}
		secure := []string{}
		for _, endpoint := range service[1] {
			u, err := url.Parse(endpoint)
			if err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
				secure = append(secure, endpoint)
			}
		}
		for _, tld := range service[0] {
			tld = strings.ToLower(tld)
			if validateLabel(tld) == nil && len(secure) > 0 {
				endpoints[tld] = secure
			}
		}
	}
	if len(endpoints["com"]) == 0 {
		return nil, fmt.Errorf("invalid IANA bootstrap: missing .com service")
	}
	return endpoints, nil
}

func parseTLDs(tldText string) ([]string, error) {
	if !strings.HasPrefix(tldText, "# Version ") {
		return nil, fmt.Errorf("invalid IANA TLD list: missing version header")
	}
	seen := map[string]bool{}
	tlds := []string{}
	for _, line := range strings.Split(tldText, "\n") {
		tld := strings.ToLower(strings.TrimSpace(line))
		if tld == "" || strings.HasPrefix(tld, "#") {
			continue
		}
		if err := validateLabel(tld); err != nil {
			return nil, fmt.Errorf("invalid IANA TLD list: %w", err)
		}
		if !seen[tld] {
			tlds = append(tlds, tld)
			seen[tld] = true
		}
	}
	if !seen["com"] {
		return nil, fmt.Errorf("invalid IANA TLD list: missing .com")
	}
	sort.Strings(tlds)
	return tlds, nil
}

func (r *Registry) ensure(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded {
		return nil
	}
	endpoints, err := parseBootstrap(loadMetadata("rdap.json"))
	if err != nil {
		endpoints, err = parseBootstrap(embeddedRDAP)
		if err != nil {
			return fmt.Errorf("invalid bundled registry metadata: %w", err)
		}
	}
	tlds, err := parseTLDs(string(loadMetadata("tlds.txt")))
	if err != nil {
		tlds, err = parseTLDs(embeddedTLDs)
		if err != nil {
			return err
		}
	}
	r.endpoints, r.tlds = endpoints, tlds
	r.loaded = true
	return ctx.Err()
}
func (r *Registry) Refresh(ctx context.Context) error {
	metadata := map[string][]byte{}
	for _, entry := range []struct{ name, url string }{{"rdap.json", "https://data.iana.org/rdap/dns.json"}, {"tlds.txt", "https://data.iana.org/TLD/tlds-alpha-by-domain.txt"}} {
		req, _ := http.NewRequestWithContext(ctx, "GET", entry.url, nil)
		resp, err := r.client.Do(req)
		if err != nil {
			return err
		}
		b, err := readLimited(resp.Body, maxResponse)
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("IANA returned HTTP %d", resp.StatusCode)
		}
		metadata[entry.name] = b
	}
	endpoints, err := parseBootstrap(metadata["rdap.json"])
	if err != nil {
		return err
	}
	tlds, err := parseTLDs(string(metadata["tlds.txt"]))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Fresh data remains usable even when the cache directory is read-only.
	r.mu.Lock()
	r.endpoints, r.tlds, r.loaded = endpoints, tlds, true
	r.mu.Unlock()
	for name, b := range metadata {
		cacheMetadata(name, b)
	}
	return nil
}
func validateLabel(label string) error {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return fmt.Errorf("domain labels must contain 1–63 characters and cannot start/end with a hyphen")
	}
	for _, c := range label {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return fmt.Errorf("invalid character in domain name")
		}
	}
	return nil
}
func domainASCII(input string) (string, error) {
	if strings.Contains(input, "://") {
		host, err := hostname(input)
		if err != nil {
			return "", err
		}
		input = host
	}
	input = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(input)), ".")
	ascii, err := idna.Lookup.ToASCII(input)
	if err != nil {
		return "", err
	}
	ascii = strings.TrimSuffix(ascii, ".")
	if len(ascii) > 253 {
		return "", fmt.Errorf("domain name is too long")
	}
	for _, label := range strings.Split(ascii, ".") {
		if err = validateLabel(label); err != nil {
			return "", err
		}
	}
	return ascii, nil
}
func registrable(input string) (string, error) {
	domain, err := domainASCII(input)
	if err != nil {
		return "", err
	}
	if !strings.Contains(domain, ".") {
		return "", fmt.Errorf("enter a full domain, such as example.com")
	}
	// Private hosting suffixes are not public registration boundaries.
	probe := domain
	publicSuffix := ""
	for {
		suffix, icann := publicsuffix.PublicSuffix(probe)
		if icann {
			publicSuffix = suffix
			break
		}
		index := strings.Index(probe, ".")
		if index < 0 {
			return "", fmt.Errorf("domain has no recognized public suffix")
		}
		probe = probe[index+1:]
	}
	prefix := strings.TrimSuffix(domain, "."+publicSuffix)
	if prefix == domain || prefix == "" {
		return "", fmt.Errorf("enter a registrable domain above .%s", publicSuffix)
	}
	label := prefix[strings.LastIndex(prefix, ".")+1:]
	return label + "." + publicSuffix, nil
}
func (r *Registry) endpoint(domain string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	parts := strings.Split(domain, ".")
	for i := 0; i < len(parts); i++ {
		if urls := r.endpoints[strings.Join(parts[i:], ".")]; len(urls) > 0 {
			return strings.TrimRight(urls[0], "/")
		}
	}
	return ""
}
func (r *Registry) acquire(ctx context.Context, endpoint string) (func(), error) {
	u, _ := url.Parse(endpoint)
	r.mu.Lock()
	if until := r.cooldowns[u.Host]; time.Now().Before(until) {
		r.mu.Unlock()
		return nil, fmt.Errorf("registry rate limited; retry after %s", until.UTC().Format(time.RFC3339))
	}
	if r.gates == nil {
		r.gates = map[string]chan struct{}{}
	}
	gate, ok := r.gates[u.Host]
	if !ok {
		gate = make(chan struct{}, 1)
		r.gates[u.Host] = gate
	}
	r.mu.Unlock()
	select {
	case gate <- struct{}{}:
		release := func() { time.AfterFunc(200*time.Millisecond, func() { <-gate }) }
		r.mu.Lock()
		until := r.cooldowns[u.Host]
		r.mu.Unlock()
		if time.Now().Before(until) {
			release()
			return nil, fmt.Errorf("registry rate limited; retry after %s", until.UTC().Format(time.RFC3339))
		}
		return release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *Registry) rateLimit(endpoint, retryAfter string) time.Time {
	now := time.Now()
	until := now.Add(time.Minute)
	if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil && seconds >= 0 {
		until = now.Add(time.Duration(min(seconds, 86400)) * time.Second)
	} else if date, err := http.ParseTime(retryAfter); err == nil {
		until = date
	}
	until = maxTime(now.Add(time.Second), minTime(until, now.Add(24*time.Hour)))
	u, _ := url.Parse(endpoint)
	r.mu.Lock()
	if r.cooldowns == nil {
		r.cooldowns = map[string]time.Time{}
	}
	r.cooldowns[u.Host] = until
	r.mu.Unlock()
	return until
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// Interleave registries so one provider's queue does not occupy every worker.
// Results still retain the caller's original TLD order.
func (r *Registry) lookupOrder(domains []string) []int {
	groups := map[string][]int{}
	keys := []string{}
	for index, domain := range domains {
		u, _ := url.Parse(r.endpoint(domain))
		host := u.Host
		if _, ok := groups[host]; !ok {
			keys = append(keys, host)
		}
		groups[host] = append(groups[host], index)
	}
	order := make([]int, 0, len(domains))
	for len(order) < len(domains) {
		for _, host := range keys {
			if group := groups[host]; len(group) > 0 {
				order = append(order, group[0])
				groups[host] = group[1:]
			}
		}
	}
	return order
}

type RDAPLookup struct {
	Domain    string         `json:"domain"`
	State     string         `json:"state"`
	Protocol  string         `json:"protocol,omitempty"`
	Source    string         `json:"source,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Registrar string         `json:"registrar,omitempty"`
	Raw       map[string]any `json:"record,omitempty"`
}

// Lookup checks the authoritative RDAP service, or the registry's legacy
// WHOIS server when IANA publishes no RDAP service for the TLD.
func (r *Registry) Lookup(ctx context.Context, domain string) RDAPLookup {
	result := r.lookupRDAP(ctx, domain)
	if result.Protocol == "" && result.State == "unknown" && ctx.Err() == nil {
		tld := domain[strings.LastIndex(domain, ".")+1:]
		if server, _ := r.whoisServer(ctx, tld, false); server != "" {
			return r.lookupWhois(ctx, domain, server)
		}
		result.Reason = "No RDAP or WHOIS service published by IANA for ." + tld
	}
	return result
}

func (r *Registry) lookupRDAP(ctx context.Context, domain string) RDAPLookup {
	result := RDAPLookup{Domain: domain, State: "unknown"}
	if err := r.ensure(ctx); err != nil {
		result.Reason = err.Error()
		result.Protocol = "rdap"
		return result
	}
	endpoint := r.endpoint(domain)
	if endpoint == "" {
		return result
	}
	result.Protocol = "rdap"
	result.Source = endpoint
	release, err := r.acquire(ctx, endpoint)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	defer release()
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(queryCtx, "GET", endpoint+"/domain/"+url.PathEscape(domain), nil)
	req.Header.Set("Accept", "application/rdap+json, application/json")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := r.client.Do(req)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		until := r.rateLimit(endpoint, resp.Header.Get("Retry-After"))
		result.Reason = "Registry returned HTTP 429; retry after " + until.UTC().Format(time.RFC3339)
		return result
	}
	b, err := readLimited(resp.Body, maxResponse)
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	var data map[string]any
	jsonErr := json.Unmarshal(b, &data)
	if resp.StatusCode == 404 {
		requested, _ := url.Parse(endpoint)
		if resp.Request != nil && (resp.Request.URL.Scheme != "https" || !strings.EqualFold(resp.Request.URL.Host, requested.Host)) {
			result.Reason = "Not-found response came from a redirected service; registration status is inconclusive"
			return result
		}
		code, _ := data["errorCode"].(float64)
		mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		emptyRDAP := len(strings.TrimSpace(string(b))) == 0 && strings.EqualFold(mediaType, "application/rdap+json")
		if (jsonErr == nil && code == 404) || emptyRDAP {
			result.State = "unregistered"
			result.Reason = "Authoritative registry reports no registration record"
		} else {
			result.Reason = "HTTP 404 without a valid RDAP not-found response"
		}
		return result
	}
	if jsonErr != nil {
		result.Reason = fmt.Sprintf("HTTP %d with invalid RDAP JSON", resp.StatusCode)
		return result
	}
	if resp.StatusCode != 200 {
		result.Reason = fmt.Sprintf("Registry returned HTTP %d", resp.StatusCode)
		if resp.StatusCode == 429 {
			result.Reason += " (rate limited)"
		}
		return result
	}
	class, _ := data["objectClassName"].(string)
	name, _ := data["ldhName"].(string)
	if class != "domain" || !strings.EqualFold(strings.TrimSuffix(name, "."), domain) {
		result.Reason = "RDAP response does not match the queried domain"
		return result
	}
	result.State = "registered"
	result.Raw = data
	return result
}
func (e *Engine) Domains(ctx context.Context, req Request, emit Emit) (Result, error) {
	name, err := domainASCII(req.Input)
	if err != nil {
		return Result{}, err
	}
	if err = e.RDAP.ensure(ctx); err != nil {
		return Result{}, err
	}
	mode := req.Opt("tlds", "popular")
	if req.Bool("all") {
		mode = "all"
	}
	if req.Bool("refresh") {
		if err = e.RDAP.Refresh(ctx); err != nil {
			return Result{}, err
		}
	}
	domains := []string{}
	if strings.Contains(name, ".") {
		if mode != "popular" && mode != "" {
			return Result{}, fmt.Errorf("use a bare name with --tlds or --all; use a full domain for an exact lookup")
		}
		root, err := registrable(name)
		if err != nil {
			return Result{}, err
		}
		if root != name {
			return Result{}, fmt.Errorf("check the registrable domain %s", root)
		}
		domains = []string{name}
	} else {
		tlds := []string{"com", "net", "org", "io", "dev", "app", "ai", "co", "me", "xyz", "tech", "cloud", "site", "online", "store", "sh"}
		if mode == "all" {
			e.RDAP.mu.Lock()
			tlds = append([]string(nil), e.RDAP.tlds...)
			e.RDAP.mu.Unlock()
		} else if mode != "popular" {
			tlds = nil
			for _, v := range strings.Split(mode, ",") {
				v, err = domainASCII(strings.TrimPrefix(strings.TrimSpace(v), "."))
				if err != nil {
					return Result{}, fmt.Errorf("invalid TLD: %w", err)
				}
				tlds = append(tlds, v)
			}
		}
		seen := map[string]bool{}
		for _, tld := range tlds {
			domain := name + "." + tld
			if len(domain) > 253 || seen[domain] {
				continue
			}
			seen[domain] = true
			domains = append(domains, domain)
		}
	}
	if len(domains) == 0 {
		return Result{}, fmt.Errorf("no domains to check")
	}
	workers, err := req.Int("concurrency", 6, 1, 12)
	if err != nil {
		return Result{}, err
	}
	jobs := make(chan int)
	results := make([]RDAPLookup, len(domains))
	var done atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					results[index] = RDAPLookup{Domain: domains[index], State: "unknown", Reason: "cancelled"}
					continue
				}
				result := e.RDAP.Lookup(ctx, domains[index])
				results[index] = result
				n := done.Add(1)
				report(emit, Progress{Message: fmt.Sprintf("%d / %d checked · %s · %s", n, len(domains), result.Domain, result.State), Fraction: float64(n) / float64(len(domains))})
			}
		}()
	}
	for _, i := range e.RDAP.lookupOrder(domains) {
		select {
		case jobs <- i:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return Result{}, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	counts := map[string]int{"registered": 0, "unregistered": 0, "unknown": 0}
	table := &Table{Headers: []string{"DOMAIN", "STATUS", "DETAIL"}}
	for _, r := range results {
		counts[r.State]++
		if req.Bool("only-unregistered") && r.State != "unregistered" {
			continue
		}
		detail := r.Reason
		if r.State == "registered" {
			detail = nonempty(r.Registrar, "Registry record found")
			if r.Raw != nil {
				detail = rdapRegistrar(r.Raw)
			}
		}
		if r.Protocol == "whois" && !strings.Contains(detail, "WHOIS") {
			detail += " (WHOIS)"
		}
		table.Rows = append(table.Rows, []string{r.Domain, r.State, detail})
	}
	return Result{Title: "Domain availability", Summary: fmt.Sprintf("%d checked · %d unregistered · %d registered · %d unknown", len(results), counts["unregistered"], counts["registered"], counts["unknown"]), Metrics: []Metric{{Label: "UNREGISTERED", Value: fmt.Sprint(counts["unregistered"])}, {Label: "REGISTERED", Value: fmt.Sprint(counts["registered"])}, {Label: "UNKNOWN", Value: fmt.Sprint(counts["unknown"])}}, Table: table, Data: map[string]any{"counts": counts, "domains": results}, Notes: []string{"Unregistered means the authoritative registry has no registration record (RDAP, or legacy WHOIS for TLDs without RDAP). Reserved, premium, brand and restricted names may still be unavailable to register. Confirm with a registrar.", "All-TLD mode covers IANA's root-zone TLD list. Unrecognized WHOIS replies, rate limits and network failures are marked unknown. Multi-label suffixes can be supplied explicitly with --tlds.", "Only public IANA metadata is cached. Domain queries and results are not saved. Run with --refresh to update the bundled registry list."}}, nil
}
func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func stringList(v any) []string {
	a, _ := v.([]any)
	out := []string{}
	for _, x := range a {
		if s := str(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}
func rdapContacts(data map[string]any) []Row {
	rows := []Row{}
	var visit func([]any, int)
	visit = func(entities []any, depth int) {
		if depth > 4 {
			return
		}
		for _, item := range entities {
			entity, ok := item.(map[string]any)
			if !ok {
				continue
			}
			roles := strings.Join(stringList(entity["roles"]), ", ")
			card, _ := entity["vcardArray"].([]any)
			if len(card) == 2 {
				properties, _ := card[1].([]any)
				for _, p := range properties {
					parts, _ := p.([]any)
					if len(parts) < 4 {
						continue
					}
					key := str(parts[0])
					if key == "fn" || key == "org" || key == "email" || key == "tel" || key == "adr" {
						val := str(parts[3])
						if val == "" {
							val = strings.Join(stringList(parts[3]), ", ")
						}
						if val != "" {
							rows = append(rows, row(nonempty(roles, "entity")+" · "+key, val))
						}
					}
				}
			}
			nested, _ := entity["entities"].([]any)
			visit(nested, depth+1)
		}
	}
	entities, _ := data["entities"].([]any)
	visit(entities, 0)
	return rows
}
func rdapRegistrar(data map[string]any) string {
	for _, r := range rdapContacts(data) {
		if strings.Contains(r.Label, "registrar") && strings.HasSuffix(r.Label, "fn") {
			return r.Value
		}
	}
	return "Registry record found"
}
func rdapSections(d RDAPLookup) []Section {
	data := d.Raw
	rows := []Row{row("Domain", d.Domain), row("Handle", str(data["handle"])), row("Registrar", rdapRegistrar(data)), row("Status", strings.Join(stringList(data["status"]), ", ")), row("Registry RDAP", d.Source)}
	events, _ := data["events"].([]any)
	for _, item := range events {
		event, _ := item.(map[string]any)
		if event != nil {
			rows = append(rows, row(str(event["eventAction"]), str(event["eventDate"])))
		}
	}
	ns, _ := data["nameservers"].([]any)
	names := []string{}
	for _, item := range ns {
		v, _ := item.(map[string]any)
		if v != nil {
			names = append(names, str(v["ldhName"]))
		}
	}
	rows = append(rows, row("Nameservers", strings.Join(names, ", ")))
	if secure, ok := data["secureDNS"].(map[string]any); ok {
		rows = append(rows, row("DNSSEC signed", secure["delegationSigned"]))
	}
	sections := []Section{{Title: "REGISTRATION", Rows: rows}}
	contacts := rdapContacts(data)
	if len(contacts) > 0 {
		sections = append(sections, Section{Title: "PUBLIC CONTACTS", Rows: contacts})
	}
	notices, _ := data["notices"].([]any)
	for _, item := range notices {
		v, _ := item.(map[string]any)
		if v != nil {
			sections = append(sections, Section{Title: nonempty(str(v["title"]), "REGISTRY NOTICE"), Text: strings.Join(stringList(v["description"]), "\n")})
		}
	}
	return sections
}
func (e *Engine) Whois(ctx context.Context, r Request) (Result, error) {
	domain, err := registrable(r.Input)
	if err != nil {
		return Result{}, err
	}
	lookup := e.RDAP.lookupRDAP(ctx, domain)
	if lookup.State == "registered" {
		result := Result{Title: "WHOIS / RDAP", Summary: domain, Sections: rdapSections(lookup), Data: lookup.Raw, Notes: []string{"Registration data comes from the authoritative registry. Privacy-protected contact details may be redacted."}}
		links, _ := lookup.Raw["links"].([]any)
		for _, item := range links {
			link, _ := item.(map[string]any)
			if link == nil || str(link["rel"]) != "related" {
				continue
			}
			u, parseErr := url.Parse(str(link["href"]))
			if parseErr != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || !strings.Contains(strings.ToLower(u.Path), "/domain/") {
				continue
			}
			var registrar map[string]any
			fetchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			fetchErr := e.getJSON(fetchCtx, u.String(), &registrar)
			cancel()
			if fetchErr != nil {
				result.Notes = append(result.Notes, "Registrar detail lookup: "+fetchErr.Error())
				break
			}
			if str(registrar["objectClassName"]) != "domain" || !strings.EqualFold(strings.TrimSuffix(str(registrar["ldhName"]), "."), domain) {
				result.Notes = append(result.Notes, "Registrar detail response did not match this domain.")
				break
			}
			contacts := rdapContacts(registrar)
			if len(contacts) > 0 {
				result.Sections = append(result.Sections, Section{Title: "REGISTRAR CONTACT DETAILS", Rows: contacts})
			}
			lookup.Raw["registrar_record"] = registrar
			result.Sections = append(result.Sections, Section{Title: "REGISTRAR SOURCE", Rows: []Row{row("RDAP", u.String())}})
			break
		}
		if r.Bool("raw") {
			result.Output = PrettyJSON(lookup.Raw)
		}
		return result, nil
	}
	if lookup.State == "unregistered" {
		return Result{Title: "WHOIS / RDAP", Summary: domain + " · no registration record", Output: lookup.Reason, Data: lookup, Notes: []string{"A missing registration record does not guarantee a name is purchasable."}}, nil
	}
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	reason := nonempty(lookup.Reason, "no RDAP service published by IANA")
	tld := domain[strings.LastIndex(domain, ".")+1:]
	server, err := e.RDAP.whoisServer(ctx, tld, true)
	if err != nil {
		return Result{}, fmt.Errorf("RDAP inconclusive (%s); WHOIS fallback unavailable: %w", reason, err)
	}
	if server == "" {
		return Result{}, fmt.Errorf("RDAP inconclusive (%s); IANA has no legacy WHOIS server for .%s", reason, tld)
	}
	release, err := e.RDAP.acquire(ctx, "whois://"+server)
	if err != nil {
		return Result{}, err
	}
	text, err := e.RDAP.whoisQuery(ctx, server, domain)
	release()
	if err != nil {
		return Result{}, err
	}
	state, detail := classifyWhois(domain, text)
	rows := []Row{row("Domain", domain), row("Status", state), row("Detail", detail), row("WHOIS server", server)}
	for _, field := range []struct {
		label string
		keys  []string
	}{
		{"Registrar", []string{"registrar", "registrar name", "sponsoring registrar", "registrar organization"}},
		{"Created", []string{"creation date", "created", "created on", "registered", "registered on", "registration date", "record created"}},
		{"Updated", []string{"updated date", "last updated", "changed", "last modified", "modified"}},
		{"Expires", []string{"registry expiry date", "registrar registration expiration date", "expiration date", "expiry date", "expires", "expires on", "paid-till"}},
	} {
		if v := whoisField(text, field.keys...); v != "" {
			rows = append(rows, row(field.label, v))
		}
	}
	return Result{Title: "Legacy WHOIS", Summary: domain + " · " + state + " · " + server, Sections: []Section{{Title: "REGISTRATION", Rows: rows}, {Title: "WHOIS RESPONSE", Text: text}}, Data: map[string]string{"domain": domain, "state": state, "server": server, "whois": text}, Notes: []string{"Legacy WHOIS uses unencrypted TCP port 43. Returned fields depend on the registry.", "RDAP was unavailable: " + reason}}, nil
}
