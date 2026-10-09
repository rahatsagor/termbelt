package core

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/idna"
)

const maxResponse = 4 << 20

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func parseHTTPURL(input string) (*url.URL, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("enter a hostname or URL")
	}
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}
	u, err := url.Parse(input)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https URLs are supported")
	}
	if u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("URL must have a host and no embedded credentials")
	}
	host, err := idna.Lookup.ToASCII(u.Hostname())
	if err != nil {
		return nil, fmt.Errorf("invalid hostname: %w", err)
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return nil, fmt.Errorf("invalid hostname")
	}
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("port must be 1–65535")
		}
		u.Host = net.JoinHostPort(host, u.Port())
	} else if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	return u, nil
}
func hostname(input string) (string, error) {
	u, err := parseHTTPURL(input)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), nil
}
func readLimited(body io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return b, nil
}
func (e *Engine) get(ctx context.Context, address string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json, application/rdap+json, text/plain, */*")
	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := readLimited(resp.Body, maxResponse)
	return b, resp.StatusCode, err
}
func (e *Engine) getJSON(ctx context.Context, address string, out any) error {
	b, status, err := e.get(ctx, address)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("provider returned HTTP %d", status)
	}
	if err = json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("invalid provider response: %w", err)
	}
	return nil
}

type IPData struct {
	IP           string  `json:"ip"`
	City         string  `json:"city,omitempty"`
	Region       string  `json:"region,omitempty"`
	Country      string  `json:"country,omitempty"`
	CountryCode  string  `json:"country_code,omitempty"`
	Continent    string  `json:"continent,omitempty"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Timezone     string  `json:"timezone,omitempty"`
	Postal       string  `json:"postal,omitempty"`
	ISP          string  `json:"isp,omitempty"`
	Organization string  `json:"organization,omitempty"`
	ASN          string  `json:"asn,omitempty"`
	Source       string  `json:"source"`
}

func formatASN(asn int) string {
	if asn <= 0 {
		return ""
	}
	return fmt.Sprintf("AS%d", asn)
}

// Shared, documentation, benchmarking and reserved ranges are not routed on the
// public internet, so geolocation providers have nothing meaningful to say.
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("100::/64"),
}

func localIPKind(ip netip.Addr) string {
	switch {
	case ip.IsLoopback():
		return "Loopback address"
	case ip.IsPrivate():
		return "Private network"
	case ip.IsLinkLocalUnicast():
		return "Link-local address"
	case ip.IsMulticast(), ip.IsLinkLocalMulticast():
		return "Multicast address"
	case ip.IsUnspecified():
		return "Unspecified address"
	case netip.MustParsePrefix("100.64.0.0/10").Contains(ip):
		return "Carrier-grade NAT shared address"
	default:
		return "Reserved or documentation address"
	}
}

func isLocalIP(ip netip.Addr) bool {
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
func (e *Engine) lookupIP(ctx context.Context, input string) (IPData, error) {
	input = strings.TrimSpace(input)
	if input != "" {
		ip, err := netip.ParseAddr(input)
		if err != nil {
			host, err := hostname(input)
			if err != nil {
				return IPData{}, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return IPData{}, fmt.Errorf("could not resolve %s", host)
			}
			ip = ips[0]
			for _, v := range ips {
				if v.Is4() {
					ip = v
					break
				}
			}
		}
		ip = ip.Unmap()
		input = ip.String()
		if isLocalIP(ip) {
			return IPData{IP: ip.String(), Source: "local classification", Organization: localIPKind(ip)}, nil
		}
	}
	if e.Config.IPAPIURL != "" {
		if _, err := ValidateIPAPI(e.Config.IPAPIURL); err != nil {
			return IPData{}, err
		}
		base := strings.TrimRight(e.Config.IPAPIURL, "/")
		path := "/me"
		if input != "" {
			path = "/ip/" + url.PathEscape(input)
		}
		var d struct {
			IP, City, Region, Country, CountryCode, Continent, Timezone, Postal, ISP, Org, ASN string
			Latitude, Longitude                                                                float64
		}
		if err := e.getJSON(ctx, base+path, &d); err != nil {
			return IPData{}, fmt.Errorf("configured IP API: %w", err)
		}
		if err := validateProviderIP(d.IP, input); err != nil {
			return IPData{}, fmt.Errorf("configured IP API: %w", err)
		}
		return IPData{IP: d.IP, City: d.City, Region: d.Region, Country: d.Country, CountryCode: d.CountryCode, Continent: d.Continent, Latitude: d.Latitude, Longitude: d.Longitude, Timezone: d.Timezone, Postal: d.Postal, ISP: d.ISP, Organization: d.Org, ASN: d.ASN, Source: base}, nil
	}
	var d struct {
		IP, City, Region, Country, Continent, Postal, Message string
		CountryCode                                           string `json:"country_code"`
		Success                                               bool
		Latitude, Longitude                                   float64
		Timezone                                              struct{ ID string }
		Connection                                            struct {
			ASN      int
			Org, ISP string
		}
	}
	err := e.getJSON(ctx, "https://ipwho.is/"+url.PathEscape(input), &d)
	if err == nil && !d.Success {
		err = fmt.Errorf("ipwho.is: %s", d.Message)
	}
	if err == nil {
		err = validateProviderIP(d.IP, input)
	}
	if err == nil {
		asn := formatASN(d.Connection.ASN)
		return IPData{IP: d.IP, City: d.City, Region: d.Region, Country: d.Country, CountryCode: d.CountryCode, Continent: d.Continent, Latitude: d.Latitude, Longitude: d.Longitude, Timezone: d.Timezone.ID, Postal: d.Postal, ISP: d.Connection.ISP, Organization: d.Connection.Org, ASN: asn, Source: "https://ipwho.is"}, nil
	}
	if ctx.Err() != nil {
		return IPData{}, ctx.Err()
	}
	var fallback struct {
		IP, CityName, Country, Continent, ZipCode, ASNOrganization string
		RegionName                                                 string `json:"regionName"`
		CountryName                                                string `json:"countryName"`
		IPAddr                                                     string `json:"ipAddress"`
		Latitude, Longitude                                        float64
		ASN                                                        int
		TimeZones                                                  []string
	}
	fallbackErr := e.getJSON(ctx, "https://free.freeipapi.com/api/json/"+url.PathEscape(input), &fallback)
	if fallbackErr != nil {
		return IPData{}, fmt.Errorf("IP providers unavailable: %v; fallback: %w", err, fallbackErr)
	}
	if parseErr := validateProviderIP(fallback.IPAddr, input); parseErr != nil {
		return IPData{}, fmt.Errorf("fallback: %w", parseErr)
	}
	zone := ""
	if len(fallback.TimeZones) > 0 {
		zone = fallback.TimeZones[0]
	}
	return IPData{IP: fallback.IPAddr, City: fallback.CityName, Region: fallback.RegionName, Country: fallback.CountryName, Continent: fallback.Continent, Postal: fallback.ZipCode, Latitude: fallback.Latitude, Longitude: fallback.Longitude, Timezone: zone, Organization: fallback.ASNOrganization, ASN: formatASN(fallback.ASN), Source: "https://free.freeipapi.com"}, nil
}

func validateProviderIP(returned, requested string) error {
	ip, err := netip.ParseAddr(returned)
	if err != nil || ip.Zone() != "" {
		return fmt.Errorf("provider returned an invalid IP")
	}
	if requested != "" {
		want, err := netip.ParseAddr(requested)
		if err != nil || ip.Unmap() != want.Unmap() {
			return fmt.Errorf("provider returned an IP that does not match the query")
		}
	}
	return nil
}
func ipRows(d IPData) []Row {
	return []Row{row("IP address", d.IP), row("Location", strings.Trim(strings.Join([]string{d.City, d.Region, d.Country}, ", "), ", ")), row("ISP", nonempty(d.ISP, "not provided")), row("Organization", nonempty(d.Organization, "not provided")), row("ASN", nonempty(d.ASN, "not provided")), row("Timezone", d.Timezone), row("Coordinates", fmt.Sprintf("%.5f, %.5f", d.Latitude, d.Longitude)), row("Source", d.Source)}
}
func (e *Engine) IP(ctx context.Context, r Request) (Result, error) {
	d, err := e.lookupIP(ctx, r.Input)
	if err != nil {
		return Result{}, err
	}
	notes := []string{"IP geolocation is approximate; a VPN or proxy changes the observed location."}
	if d.Source == "local classification" {
		notes = []string{"This address is not publicly routable, so it was classified locally and not sent to a geolocation service."}
	}
	return Result{Title: "IP intelligence", Summary: d.IP, Sections: []Section{{Title: "NETWORK & LOCATION", Rows: ipRows(d)}}, Notes: notes, Data: d}, nil
}

type HTTPData struct {
	URL       string             `json:"url"`
	Status    int                `json:"status"`
	Protocol  string             `json:"protocol"`
	Headers   http.Header        `json:"headers"`
	Redirects []string           `json:"redirects"`
	Timings   map[string]float64 `json:"timings_ms"`
	Bytes     int                `json:"bytes_read"`
	Truncated bool               `json:"body_truncated"`
	Body      string             `json:"-"`
}

func (e *Engine) inspectHTTP(ctx context.Context, input string) (HTTPData, error) {
	u, err := parseHTTPURL(input)
	if err != nil {
		return HTTPData{}, err
	}
	start := time.Now()
	timings := map[string]float64{}
	var mu sync.Mutex
	var dnsStart, connectStart, tlsStart time.Time
	trace := &httptrace.ClientTrace{DNSStart: func(httptrace.DNSStartInfo) { mu.Lock(); dnsStart = time.Now(); mu.Unlock() }, DNSDone: func(httptrace.DNSDoneInfo) {
		mu.Lock()
		if !dnsStart.IsZero() {
			timings["dns"] = float64(time.Since(dnsStart).Microseconds()) / 1000
		}
		mu.Unlock()
	}, ConnectStart: func(string, string) { mu.Lock(); connectStart = time.Now(); mu.Unlock() }, ConnectDone: func(string, string, error) {
		mu.Lock()
		if !connectStart.IsZero() {
			timings["tcp"] = float64(time.Since(connectStart).Microseconds()) / 1000
		}
		mu.Unlock()
	}, TLSHandshakeStart: func() { mu.Lock(); tlsStart = time.Now(); mu.Unlock() }, TLSHandshakeDone: func(tls.ConnectionState, error) {
		mu.Lock()
		if !tlsStart.IsZero() {
			timings["tls"] = float64(time.Since(tlsStart).Microseconds()) / 1000
		}
		mu.Unlock()
	}, GotFirstResponseByte: func() { mu.Lock(); timings["ttfb"] = float64(time.Since(start).Microseconds()) / 1000; mu.Unlock() }}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, u.String(), nil)
	if err != nil {
		return HTTPData{}, err
	}
	req.Header.Set("User-Agent", UserAgent)
	redirects := []string{}
	client := *e.Client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("more than 10 redirects")
		}
		redirects = append(redirects, req.URL.String())
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return HTTPData{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return HTTPData{}, err
	}
	truncated := len(b) > 1<<20
	if truncated {
		b = b[:1<<20]
	}
	mu.Lock()
	timings["total"] = float64(time.Since(start).Microseconds()) / 1000
	mu.Unlock()
	return HTTPData{URL: resp.Request.URL.String(), Status: resp.StatusCode, Protocol: resp.Proto, Headers: resp.Header, Redirects: redirects, Timings: timings, Bytes: len(b), Truncated: truncated, Body: string(b)}, nil
}
func httpSections(d HTTPData) []Section {
	rows := []Row{row("URL", d.URL), row("Status", fmt.Sprintf("%d %s", d.Status, http.StatusText(d.Status))), row("Protocol", d.Protocol), row("Body read", fmt.Sprintf("%d bytes", d.Bytes))}
	if d.Truncated {
		rows = append(rows, row("Body", "truncated at 1 MiB"))
	}
	for _, key := range sortedKeys(d.Timings) {
		rows = append(rows, row(strings.ToUpper(key), fmt.Sprintf("%.2f ms", d.Timings[key])))
	}
	headers := []Row{}
	for _, k := range sortedKeys(d.Headers) {
		headers = append(headers, row(k, strings.Join(d.Headers[k], ", ")))
	}
	sections := []Section{{Title: "REQUEST", Rows: rows}, {Title: "RESPONSE HEADERS", Rows: headers}}
	if len(d.Redirects) > 0 {
		sections = append(sections, Section{Title: "REDIRECTS", Text: strings.Join(d.Redirects, "\n")})
	}
	return sections
}
func (e *Engine) HTTP(ctx context.Context, r Request) (Result, error) {
	d, err := e.inspectHTTP(ctx, r.Input)
	if err != nil {
		return Result{}, err
	}
	return Result{Title: "HTTP inspector", Summary: fmt.Sprintf("%d · %s", d.Status, d.URL), Sections: httpSections(d), Data: d, Notes: []string{"Timings describe this request path, including redirects. DNS/TCP/TLS timings may be absent for reused connections."}}, nil
}

type DNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	TTL   uint32 `json:"ttl"`
	Value string `json:"value"`
}

func dnsResolver(custom string) (string, error) {
	custom = strings.TrimSpace(custom)
	if custom != "" {
		host := strings.Trim(custom, "[]")
		if h, p, err := net.SplitHostPort(custom); err == nil {
			if _, err = netip.ParseAddr(h); err != nil {
				return "", fmt.Errorf("resolver must be an IP address")
			}
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > 65535 {
				return "", fmt.Errorf("invalid resolver port")
			}
			return net.JoinHostPort(h, p), nil
		}
		if _, err := netip.ParseAddr(host); err != nil {
			return "", fmt.Errorf("resolver must be an IP address, optionally with a port")
		}
		return net.JoinHostPort(host, "53"), nil
	}
	if servers := systemResolvers(); len(servers) > 0 {
		return servers[0], nil
	}
	return "", fmt.Errorf("could not read the system DNS resolver; specify --resolver 1.1.1.1")
}

func resolvConfServers() []string {
	cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	servers := []string{}
	for _, server := range cfg.Servers {
		if ip, err := netip.ParseAddr(server); err == nil {
			servers = append(servers, net.JoinHostPort(ip.String(), cfg.Port))
		}
	}
	return servers
}
func queryDNS(ctx context.Context, host string, kind uint16, resolver string) ([]DNSRecord, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(host), kind)
	msg.SetEdns0(1232, false)
	client := &dns.Client{Timeout: 5 * time.Second}
	reply, _, err := client.ExchangeContext(ctx, msg, resolver)
	if err != nil {
		return nil, err
	}
	if reply.Truncated {
		client.Net = "tcp"
		reply, _, err = client.ExchangeContext(ctx, msg, resolver)
		if err != nil {
			return nil, err
		}
	}
	if reply.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DNS %s", dns.RcodeToString[reply.Rcode])
	}
	records := []DNSRecord{}
	for _, rr := range reply.Answer {
		header := rr.Header()
		value := strings.TrimSpace(strings.TrimPrefix(rr.String(), header.String()))
		records = append(records, DNSRecord{Type: dns.TypeToString[header.Rrtype], Name: header.Name, TTL: header.Ttl, Value: value})
	}
	return records, nil
}
func (e *Engine) DNS(ctx context.Context, r Request) (Result, error) {
	var host string
	var err error
	reverse := false
	if ip, parseErr := netip.ParseAddr(strings.Trim(strings.TrimSpace(r.Input), "[]")); parseErr == nil {
		host, err = dns.ReverseAddr(ip.String())
		reverse = true
	} else {
		host, err = hostname(r.Input)
	}
	if err != nil {
		return Result{}, err
	}
	host = strings.TrimSuffix(host, ".")
	resolver, err := dnsResolver(r.Opt("resolver", ""))
	if err != nil {
		return Result{}, err
	}
	kind := strings.ToUpper(strings.TrimSpace(r.Opt("type", "ALL")))
	if reverse && kind == "ALL" {
		kind = "PTR"
	}
	types := []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeMX, dns.TypeNS, dns.TypeTXT, dns.TypeCNAME, dns.TypeSOA, dns.TypeCAA}
	if kind != "ALL" {
		t, ok := dns.StringToType[kind]
		if !ok || t == dns.TypeAXFR || t == dns.TypeIXFR || t == dns.TypeOPT {
			return Result{}, fmt.Errorf("unsupported DNS record type %q", kind)
		}
		types = []uint16{t}
	}
	type answer struct {
		records []DNSRecord
		err     error
		kind    uint16
	}
	ch := make(chan answer, len(types))
	for _, t := range types {
		go func(t uint16) { records, err := queryDNS(ctx, host, t, resolver); ch <- answer{records, err, t} }(t)
	}
	records := []DNSRecord{}
	notes := []string{}
	seen := map[DNSRecord]bool{}
	for range types {
		a := <-ch
		if a.err != nil {
			notes = append(notes, dns.TypeToString[a.kind]+": "+a.err.Error())
		} else {
			for _, record := range a.records {
				if !seen[record] {
					records = append(records, record)
					seen[record] = true
				}
			}
		}
	}
	sort.Strings(notes)
	if len(notes) == len(types) {
		return Result{}, fmt.Errorf("DNS lookup failed: %s", strings.Join(notes, "; "))
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Type == records[j].Type {
			return records[i].Value < records[j].Value
		}
		return records[i].Type < records[j].Type
	})
	table := &Table{Headers: []string{"TYPE", "TTL", "VALUE"}}
	for _, record := range records {
		table.Rows = append(table.Rows, []string{record.Type, strconv.Itoa(int(record.TTL)), record.Value})
	}
	return Result{Title: "DNS records", Summary: host + " · resolver " + resolver, Table: table, Notes: notes, Data: records}, nil
}

// splitHostPort accepts host, host:port, [IPv6]:port, bare IPv6, and http(s)
// URLs. Internationalized hostnames are converted to their ASCII form.
func splitHostPort(input string, defaultPort string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("enter a host, host:port or URL")
	}
	if strings.Contains(input, "://") || strings.Contains(input, "/") {
		u, err := parseHTTPURL(input)
		if err != nil {
			return "", "", err
		}
		p := u.Port()
		if p == "" {
			p = map[string]string{"http": "80", "https": "443"}[u.Scheme]
		}
		return strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), p, nil
	}
	host, port := input, defaultPort
	if ip, err := netip.ParseAddr(strings.Trim(input, "[]")); err == nil {
		return ip.String(), port, nil
	}
	if h, p, err := net.SplitHostPort(input); err == nil {
		host, port = h, p
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", "", fmt.Errorf("port must be 1–65535")
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return "", "", fmt.Errorf("enter a host before the port")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.String(), port, nil
	}
	ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(host, "."))
	if err != nil || ascii == "" || strings.ContainsAny(ascii, " \t\r\n?#@[]") {
		return "", "", fmt.Errorf("invalid hostname %q", host)
	}
	return strings.ToLower(ascii), port, nil
}
func (e *Engine) TLS(ctx context.Context, r Request) (Result, error) {
	host, port, err := splitHostPort(r.Input, "443")
	if err != nil {
		return Result{}, err
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: time.Duration(e.Config.TimeoutSeconds) * time.Second}, Config: &tls.Config{ServerName: host, InsecureSkipVerify: true, NextProtos: []string{"h2", "http/1.1"}}} // Inspection only: trust is verified explicitly below.
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return Result{}, fmt.Errorf("server did not present a certificate")
	}
	cert := state.PeerCertificates[0]
	intermediates := x509.NewCertPool()
	for _, v := range state.PeerCertificates[1:] {
		intermediates.AddCert(v)
	}
	_, verifyErr := cert.Verify(x509.VerifyOptions{DNSName: host, Intermediates: intermediates})
	trusted := verifyErr == nil
	sum := sha256.Sum256(cert.Raw)
	days := int(math.Floor(time.Until(cert.NotAfter).Hours() / 24))
	notes := []string{}
	verifyMessage := ""
	if verifyErr != nil {
		verifyMessage = verifyErr.Error()
		notes = append(notes, "Certificate verification failed: "+verifyMessage)
	}
	switch {
	case time.Now().After(cert.NotAfter):
		notes = append(notes, "The certificate has expired.")
	case time.Now().Before(cert.NotBefore):
		notes = append(notes, "The certificate is not valid yet.")
	case days < 14:
		notes = append(notes, fmt.Sprintf("The certificate expires in %d days; renew it soon.", days))
	}
	names := append(append([]string{}, cert.DNSNames...), ipStrings(cert.IPAddresses)...)
	alpn := nonempty(state.NegotiatedProtocol, "none")
	rows := []Row{row("Subject", cert.Subject.String()), row("Issuer", cert.Issuer.String()), row("Valid from", cert.NotBefore.Format(time.RFC3339)), row("Expires", cert.NotAfter.Format(time.RFC3339)), row("Days remaining", days), row("Trusted", trusted), row("TLS", tls.VersionName(state.Version)), row("Cipher", tls.CipherSuiteName(state.CipherSuite)), row("ALPN", alpn), row("Key", publicKeyDescription(cert)), row("Signature", cert.SignatureAlgorithm.String()), row("Serial", cert.SerialNumber.Text(16)), row("SHA-256", hex.EncodeToString(sum[:])), row("Names", strings.Join(names, ", ")), row("Chain length", len(state.PeerCertificates))}
	chain := []map[string]any{}
	chainRows := []Row{}
	for i, c := range state.PeerCertificates {
		chain = append(chain, map[string]any{"subject": c.Subject.String(), "issuer": c.Issuer.String(), "not_before": c.NotBefore, "not_after": c.NotAfter, "is_ca": c.IsCA})
		chainRows = append(chainRows, row(fmt.Sprintf("#%d", i), c.Subject.CommonName+" · expires "+c.NotAfter.Format("2006-01-02")))
	}
	return Result{Title: "TLS certificate", Summary: host + ":" + port, Metrics: []Metric{{Label: "EXPIRES IN", Value: strconv.Itoa(days), Unit: "days"}, {Label: "TRUST", Value: map[bool]string{true: "valid", false: "failed"}[trusted]}}, Sections: []Section{{Title: "CERTIFICATE", Rows: rows}, {Title: "PRESENTED CHAIN", Rows: chainRows}}, Notes: notes, Data: map[string]any{"host": host, "port": port, "trusted": trusted, "verify_error": verifyMessage, "issuer": cert.Issuer.String(), "subject": cert.Subject.String(), "not_before": cert.NotBefore, "not_after": cert.NotAfter, "days_remaining": days, "dns_names": cert.DNSNames, "ip_addresses": ipStrings(cert.IPAddresses), "serial": cert.SerialNumber.Text(16), "sha256": hex.EncodeToString(sum[:]), "tls_version": tls.VersionName(state.Version), "cipher": tls.CipherSuiteName(state.CipherSuite), "alpn": state.NegotiatedProtocol, "key": publicKeyDescription(cert), "signature_algorithm": cert.SignatureAlgorithm.String(), "chain": chain}}, nil
}

func ipStrings(ips []net.IP) []string {
	out := []string{}
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out
}

func publicKeyDescription(cert *x509.Certificate) string {
	switch key := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA %d-bit", key.N.BitLen())
	case *ecdsa.PublicKey:
		return "ECDSA " + key.Curve.Params().Name
	case ed25519.PublicKey:
		return "Ed25519"
	default:
		return cert.PublicKeyAlgorithm.String()
	}
}
func (e *Engine) Ping(ctx context.Context, r Request, emit Emit) (Result, error) {
	host, port, err := splitHostPort(r.Input, "443")
	if err != nil {
		return Result{}, err
	}
	count, err := r.Int("count", 5, 1, 50)
	if err != nil {
		return Result{}, err
	}
	samples := []float64{}
	table := &Table{Headers: []string{"#", "STATUS", "LATENCY"}}
	failed := 0
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		start := time.Now()
		conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
		ms := float64(time.Since(start).Microseconds()) / 1000
		status := "connected"
		latency := fmt.Sprintf("%.2f ms", ms)
		if err != nil {
			failed++
			status = "failed"
			latency = err.Error()
		} else {
			conn.Close()
			samples = append(samples, ms)
		}
		table.Rows = append(table.Rows, []string{strconv.Itoa(i + 1), status, latency})
		report(emit, Progress{Message: fmt.Sprintf("Probe %d of %d · %s", i+1, count, status), Fraction: float64(i+1) / float64(count)})
		if i+1 < count {
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	metrics := []Metric{{Label: "FAILED", Value: fmt.Sprintf("%.0f", float64(failed)*100/float64(count)), Unit: "%"}}
	if len(samples) > 0 {
		sort.Float64s(samples)
		sum := 0.
		for _, v := range samples {
			sum += v
		}
		metrics = append(metrics, Metric{Label: "MIN", Value: fmt.Sprintf("%.2f", samples[0]), Unit: "ms"}, Metric{Label: "AVERAGE", Value: fmt.Sprintf("%.2f", sum/float64(len(samples))), Unit: "ms"}, Metric{Label: "MAX", Value: fmt.Sprintf("%.2f", samples[len(samples)-1]), Unit: "ms"})
	}
	return Result{Title: "TCP latency", Summary: net.JoinHostPort(host, port), Metrics: metrics, Table: table, Data: map[string]any{"host": host, "port": port, "attempts": count, "failed": failed, "samples_ms": samples}, Notes: []string{"TCP probes measure connection establishment, including DNS. Failure rate describes these connection attempts."}}, nil
}

var cdnByName = []struct{ name, suffix string }{
	{"Cloudflare DNS", "ns.cloudflare.com"},
	{"Amazon Route 53", "awsdns"},
	{"Amazon CloudFront", "cloudfront.net"},
	{"Fastly", "fastly.net"},
	{"Akamai", "akamaiedge.net"},
	{"Akamai", "edgekey.net"},
	{"Vercel", "vercel-dns.com"},
	{"Netlify", "netlify.app"},
	{"GitHub Pages", "github.io"},
	{"Azure Front Door", "azurefd.net"},
	{"Google Cloud DNS", "googledomains.com"},
	{"Heroku", "herokudns.com"},
}

func nameserversContain(ns []string, suffix string) bool {
	for _, n := range ns {
		if strings.Contains(strings.ToLower(n), suffix) {
			return true
		}
	}
	return false
}

func hostingClues(d HTTPData, cname string, ns []string) []Row {
	clues := []Row{}
	header := d.Headers
	lowerBody := strings.ToLower(d.Body)
	hints := []struct {
		name     string
		match    bool
		evidence string
	}{
		{"Cloudflare", header.Get("CF-Ray") != "", "CF-Ray response header"},
		{"Vercel", header.Get("X-Vercel-Id") != "", "X-Vercel-Id response header"},
		{"Netlify", header.Get("X-Nf-Request-Id") != "", "X-Nf-Request-Id response header"},
		{"Amazon CloudFront", header.Get("X-Amz-Cf-Id") != "", "X-Amz-Cf-Id response header"},
		{"GitHub Pages", strings.Contains(strings.ToLower(header.Get("Server")), "github.com"), "Server response header"},
		{"Next.js", strings.Contains(lowerBody, "/_next/") || strings.Contains(header.Get("X-Powered-By"), "Next.js"), "Next.js asset path / X-Powered-By"},
		{"WordPress", strings.Contains(lowerBody, "/wp-content/") || strings.Contains(lowerBody, "/wp-includes/"), "WordPress asset paths"},
		{"Shopify", strings.Contains(lowerBody, "cdn.shopify.com"), "Shopify asset hostname"},
		{"Fastly", header.Get("X-Served-By") != "" && strings.Contains(header.Get("X-Served-By"), "cache-"), "X-Served-By cache header"},
		{"Akamai", strings.Contains(strings.ToLower(header.Get("Server")), "akamai"), "Server response header"},
		{"Google Cloud", strings.Contains(header.Get("Via"), "google") || header.Get("Server") == "gws" || header.Get("Server") == "Google Frontend", "Via / Server response header"},
		{"Azure", header.Get("X-Azure-Ref") != "" || header.Get("X-MSEdge-Ref") != "", "X-Azure-Ref response header"},
		{"Fly.io", header.Get("Fly-Request-Id") != "", "Fly-Request-Id response header"},
		{"Render", header.Get("Rndr-Id") != "", "Rndr-Id response header"},
		{"Nuxt", strings.Contains(lowerBody, "/_nuxt/"), "Nuxt asset path"},
		{"Gatsby", strings.Contains(lowerBody, "___gatsby"), "Gatsby root element"},
		{"Astro", strings.Contains(lowerBody, "/_astro/"), "Astro asset path"},
		{"SvelteKit", strings.Contains(lowerBody, "/_app/immutable/"), "SvelteKit asset path"},
		{"Drupal", strings.Contains(lowerBody, "/sites/default/files/") || strings.Contains(header.Get("X-Generator"), "Drupal"), "Drupal asset path / X-Generator"},
		{"Squarespace", strings.Contains(lowerBody, "static1.squarespace.com"), "Squarespace asset hostname"},
		{"Wix", strings.Contains(lowerBody, "static.wixstatic.com"), "Wix asset hostname"},
		{"Webflow", strings.Contains(lowerBody, "assets.website-files.com") || strings.Contains(lowerBody, "data-wf-site"), "Webflow markup"},
		{"Ghost", header.Get("X-Ghost-Cache-Status") != "" || strings.Contains(lowerBody, `content="ghost`), "Ghost header / generator tag"},
		{"HSTS enabled", header.Get("Strict-Transport-Security") != "", "Strict-Transport-Security header"},
		{"HTTP/3 advertised", strings.Contains(header.Get("Alt-Svc"), "h3"), "Alt-Svc response header"},
	}
	for _, h := range hints {
		if h.match {
			clues = append(clues, row(h.name, h.evidence))
		}
	}
	seen := map[string]bool{}
	for _, h := range cdnByName {
		if !seen[h.name] && ((cname != "" && strings.Contains(cname, h.suffix)) || nameserversContain(ns, h.suffix)) {
			seen[h.name] = true
			clues = append(clues, row(h.name, "DNS points to "+h.suffix))
		}
	}
	for _, key := range []string{"Server", "X-Powered-By", "Via"} {
		if v := header.Get(key); v != "" {
			clues = append(clues, row(key, v))
		}
	}
	return clues
}

var titlePattern = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func (e *Engine) Site(ctx context.Context, r Request, emit Emit) (Result, error) {
	host, err := hostname(r.Input)
	if err != nil {
		return Result{}, err
	}
	report(emit, Progress{Message: "Resolving DNS and inspecting HTTP", Fraction: .1})
	type outcome struct {
		kind  string
		value any
		err   error
	}
	ch := make(chan outcome, 4)
	go func() { d, err := e.inspectHTTP(ctx, r.Input); ch <- outcome{"http", d, err} }()
	go func() { v, err := net.DefaultResolver.LookupIPAddr(ctx, host); ch <- outcome{"ips", v, err} }()
	go func() { v, err := net.DefaultResolver.LookupCNAME(ctx, host); ch <- outcome{"cname", v, err} }()
	nsHost := host
	if root, err := registrable(host); err == nil {
		nsHost = root
	}
	go func() { v, err := net.DefaultResolver.LookupNS(ctx, nsHost); ch <- outcome{"ns", v, err} }()
	var httpData HTTPData
	ips := []net.IPAddr{}
	cname := ""
	nameservers := []string{}
	notes := []string{}
	httpOK := false
	for i := 0; i < 4; i++ {
		o := <-ch
		if o.err != nil {
			notes = append(notes, o.kind+": "+o.err.Error())
			continue
		}
		switch o.kind {
		case "http":
			httpData = o.value.(HTTPData)
			httpOK = true
		case "ips":
			ips = o.value.([]net.IPAddr)
		case "cname":
			// The resolver returns the queried name itself when no CNAME exists.
			if v := strings.ToLower(strings.TrimSuffix(o.value.(string), ".")); v != host {
				cname = v
			}
		case "ns":
			for _, n := range o.value.([]*net.NS) {
				nameservers = append(nameservers, strings.TrimSuffix(n.Host, "."))
			}
		}
	}
	if !httpOK && len(ips) == 0 {
		return Result{}, fmt.Errorf("website inspection failed: %s", strings.Join(notes, "; "))
	}
	ipStrings := []string{}
	for _, ip := range ips {
		ipStrings = append(ipStrings, ip.IP.String())
	}
	sections := []Section{{Title: "DNS", Rows: []Row{row("Host", host), row("Addresses", strings.Join(ipStrings, ", ")), row("CNAME", nonempty(cname, "none")), row("Nameservers", strings.Join(nameservers, ", "))}}}
	var network *IPData
	if len(ips) > 0 {
		report(emit, Progress{Message: "Looking up network ownership", Fraction: .7})
		ip := ips[0].IP.String()
		for _, v := range ips {
			if v.IP.To4() != nil {
				ip = v.IP.String()
				break
			}
		}
		d, err := e.lookupIP(ctx, ip)
		if err == nil {
			network = &d
			sections = append(sections, Section{Title: "OBSERVED NETWORK OWNER", Rows: ipRows(d)})
		} else {
			notes = append(notes, "IP ownership lookup: "+err.Error())
		}
	}
	if httpOK {
		clues := hostingClues(httpData, cname, nameservers)
		if len(clues) == 0 {
			clues = append(clues, row("Technology", "No recognizable evidence exposed"))
		}
		sections = append(sections, Section{Title: "HOSTING & TECHNOLOGY CLUES", Rows: clues})
		if match := titlePattern.FindStringSubmatch(httpData.Body); len(match) > 1 {
			sections = append(sections, Section{Title: "PAGE", Rows: []Row{row("Title", html.UnescapeString(strings.TrimSpace(match[1])))}})
		}
		sections = append(sections, httpSections(httpData)...)
	}
	notes = append(notes, "Network ownership identifies the public endpoint. A CDN or reverse proxy can hide the origin hosting provider.", "Technology labels are inferences from the listed HTTP / HTML evidence.")
	return Result{Title: "Website intelligence", Summary: host, Sections: sections, Notes: notes, Data: map[string]any{"host": host, "addresses": ipStrings, "cname": cname, "nameservers": nameservers, "network": network, "http": httpData}}, nil
}
