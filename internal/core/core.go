package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type FieldKind string

const (
	TextField FieldKind = ""
	IntField  FieldKind = "int"
	BoolField FieldKind = "bool"
)

// Field describes one tool option. Every field becomes both a workbench form
// control and a CLI flag (except input and pattern, which are arguments).
type Field struct {
	Key, Label, Placeholder, Default, Help string
	Kind                                   FieldKind
}

// UserAgent identifies requests; the CLI sets the release version at startup.
var UserAgent = "termbelt"

func SetVersion(version string) {
	if version = strings.TrimSpace(version); version != "" {
		UserAgent = "termbelt/" + version
	}
}

type Tool struct {
	ID, Name, Category, Description, Hint, Example string
	Local                                          bool
	Fields                                         []Field
}

var Tools = []Tool{
	{ID: "speed", Name: "Internet speed", Category: "NETWORK", Description: "Fast.com download or Cloudflare download + upload", Hint: "Measure your connection with live throughput. Tests consume network data.", Example: "termbelt speed --provider cloudflare", Fields: []Field{{Key: "provider", Label: "Provider", Default: "fast", Help: "fast or cloudflare"}, {Key: "duration", Label: "Seconds per direction", Default: "8", Help: "2–30 seconds", Kind: IntField}, {Key: "max-mb", Label: "MiB limit per direction", Default: "128", Help: "1–1024 MiB", Kind: IntField}, {Key: "download-only", Label: "Download only", Help: "Skip the Cloudflare upload measurement", Kind: BoolField}}},
	{ID: "ip", Name: "IP intelligence", Category: "NETWORK", Description: "Your public IP, geolocation, ISP and ASN", Hint: "Leave empty for your public IP. Private IPs are identified locally.", Example: "termbelt ip 8.8.8.8", Fields: []Field{{Key: "input", Label: "IP address or hostname", Placeholder: "8.8.8.8 · empty = my IP"}}},
	{ID: "domains", Name: "Domain availability", Category: "DOMAINS", Description: "Check a name across popular or every IANA TLD", Hint: "Use a name for multiple TLDs, or a full domain for an exact check. 'Unregistered' still needs registrar confirmation.", Example: "termbelt domains myidea --all", Fields: []Field{{Key: "input", Label: "Name or domain", Placeholder: "myidea or example.com"}, {Key: "tlds", Label: "TLDs", Default: "popular", Help: "popular, all, or com,io,dev,co.uk"}, {Key: "only-unregistered", Label: "Only unregistered", Help: "Show only unregistered names in the table", Kind: BoolField}, {Key: "concurrency", Label: "Parallel lookups", Default: "6", Help: "1–12, paced per registry", Kind: IntField}, {Key: "refresh", Label: "Refresh IANA metadata", Help: "Download the latest registry list first", Kind: BoolField}}},
	{ID: "whois", Name: "WHOIS / RDAP", Category: "DOMAINS", Description: "Registrar, dates, contacts, status and nameservers", Hint: "Queries authoritative registry RDAP; falls back to legacy WHOIS when needed.", Example: "termbelt whois example.com", Fields: []Field{{Key: "input", Label: "Domain", Placeholder: "example.com"}}},
	{ID: "site", Name: "Website intelligence", Category: "DOMAINS", Description: "Hosting clues, CDN, DNS, headers and technology", Hint: "Combine network ownership, DNS and HTTP evidence. A CDN can hide the origin host.", Example: "termbelt site github.com", Fields: []Field{{Key: "input", Label: "Website", Placeholder: "https://example.com"}}},
	{ID: "dns", Name: "DNS lookup", Category: "NETWORK", Description: "A, AAAA, MX, NS, TXT, CNAME, SOA and CAA records", Hint: "Queries your system DNS resolver, or a custom resolver IP.", Example: "termbelt dns example.com --type MX", Fields: []Field{{Key: "input", Label: "Domain", Placeholder: "example.com"}, {Key: "type", Label: "Record type", Default: "ALL", Help: "ALL, A, AAAA, MX, NS, TXT, CNAME, SOA, CAA, SRV, PTR, DS…"}, {Key: "resolver", Label: "Resolver (optional)", Placeholder: "system · or 1.1.1.1", Help: "Resolver IP or IP:port"}}},
	{ID: "tls", Name: "TLS certificate", Category: "NETWORK", Description: "Issuer, validity, SANs, TLS version and fingerprint", Hint: "Inspects the presented certificate and reports trust or expiry problems.", Example: "termbelt tls example.com", Fields: []Field{{Key: "input", Label: "Host or host:port", Placeholder: "example.com:443"}}},
	{ID: "http", Name: "HTTP inspector", Category: "NETWORK", Description: "Status, redirects, headers and request timings", Hint: "Sends a GET request; reads up to 1 MiB and follows up to 10 redirects.", Example: "termbelt http https://example.com", Fields: []Field{{Key: "input", Label: "URL", Placeholder: "https://example.com"}}},
	{ID: "ping", Name: "TCP latency", Category: "NETWORK", Description: "Repeated connection latency and failure rate", Hint: "Measures TCP connection time, including DNS lookup. This is a TCP probe.", Example: "termbelt ping example.com --count 5", Fields: []Field{{Key: "input", Label: "Host or host:port", Placeholder: "example.com:443"}, {Key: "count", Label: "Probe count", Default: "5", Help: "1–50", Kind: IntField}}},
	{ID: "ports", Name: "Local listening ports", Category: "NETWORK", Description: "Find which processes are using local TCP ports", Hint: "Shows listeners visible to your account. Use --port to focus on a port.", Example: "termbelt ports --port 3000", Local: true, Fields: []Field{{Key: "port", Label: "Port filter", Placeholder: "3000 · empty = all"}}},
	{ID: "json", Name: "JSON workbench", Category: "DEVELOPER", Description: "Format, validate, minify and extract JSON paths", Hint: "Paste JSON here, or pipe it into termbelt json. Preserves large numeric values.", Example: "cat data.json | termbelt json --path users.0.name", Local: true, Fields: []Field{{Key: "input", Label: "JSON", Placeholder: "{\"hello\": \"world\"}"}, {Key: "path", Label: "Extract path (optional)", Placeholder: "users.0.name or users[0][\"display.name\"]"}, {Key: "minify", Label: "Minify", Help: "Compact the JSON output", Kind: BoolField}}},
	{ID: "base64", Name: "Base64 codec", Category: "DEVELOPER", Description: "Encode or decode standard and URL-safe Base64", Hint: "Runs locally. Use --decode for decoding, --url-safe for URL-safe encoding.", Example: "printf hello | termbelt base64", Local: true, Fields: []Field{{Key: "input", Label: "Text", Placeholder: "hello or aGVsbG8="}, {Key: "mode", Label: "Mode", Default: "encode", Help: "encode or decode"}, {Key: "url-safe", Label: "URL-safe alphabet", Help: "Encode unpadded URL-safe Base64", Kind: BoolField}}},
	{ID: "url", Name: "URL codec", Category: "DEVELOPER", Description: "Encode / decode URLs and inspect components", Hint: "encode, decode, or inspect. Encoding uses percent escaping for a URL component.", Example: "termbelt url 'https://example.com?a=1' --mode inspect", Local: true, Fields: []Field{{Key: "input", Label: "Text or URL", Placeholder: "https://example.com?q=hello"}, {Key: "mode", Label: "Mode", Default: "encode", Help: "encode, decode or inspect"}}},
	{ID: "jwt", Name: "JWT inspector", Category: "DEVELOPER", Description: "Decode claims, timestamps and expiration locally", Hint: "Decodes only. Signature verification is not performed; claims are untrusted.", Example: "termbelt jwt < token.txt", Local: true, Fields: []Field{{Key: "input", Label: "JWT", Placeholder: "eyJ… . eyJ… . signature"}}},
	{ID: "hash", Name: "Hash calculator", Category: "DEVELOPER", Description: "SHA-256, SHA-512, SHA-1 and MD5 checksums", Hint: "Hash text here. Use --file to stream a file of any size without loading it into memory.", Example: "termbelt hash --file archive.zip", Local: true, Fields: []Field{{Key: "input", Label: "Text", Placeholder: "text to hash"}, {Key: "algorithm", Label: "Algorithm", Default: "sha256", Help: "sha256, sha512, sha384, sha224, sha1, md5 or all"}, {Key: "file", Label: "File (optional)", Placeholder: "path/to/file", Help: "Stream a file instead of text"}, {Key: "verify", Label: "Expected checksum (optional)", Placeholder: "hex digest", Help: "Fail unless the digest matches"}}},
	{ID: "uuid", Name: "UUID generator", Category: "DEVELOPER", Description: "Cryptographically random RFC 9562 UUIDs", Hint: "Generate version 4 UUIDs using the operating system's secure random source.", Example: "termbelt uuid --count 5", Local: true, Fields: []Field{{Key: "count", Label: "How many", Default: "1", Help: "1–100", Kind: IntField}}},
	{ID: "password", Name: "Password generator", Category: "DEVELOPER", Description: "Secure passwords with guaranteed character classes", Hint: "No generated passwords are saved. Choose 12–256 characters.", Example: "termbelt password --length 32", Local: true, Fields: []Field{{Key: "length", Label: "Length", Default: "24", Help: "12–256 characters", Kind: IntField}, {Key: "count", Label: "How many", Default: "1", Help: "1–100", Kind: IntField}, {Key: "no-symbols", Label: "Letters and digits only", Help: "Leave out symbols", Kind: BoolField}}},
	{ID: "time", Name: "Timestamp converter", Category: "DEVELOPER", Description: "Unix s / ms / us / ns ↔ dates and timezones", Hint: "Accepts exact fractional timestamps, RFC3339, or YYYY-MM-DD HH:MM:SS. Empty = now. Set the unit for historical timestamps.", Example: "termbelt time 1735689600000 --zone Asia/Dhaka", Local: true, Fields: []Field{{Key: "input", Label: "Timestamp or date", Placeholder: "1735689600 · empty = now"}, {Key: "zone", Label: "Timezone", Default: "Local", Help: "Local, UTC or IANA name"}, {Key: "unit", Label: "Timestamp unit", Default: "auto", Help: "auto, s, ms, us or ns"}}},
	{ID: "cron", Name: "Cron explorer", Category: "DEVELOPER", Description: "Preview the next scheduled runs in your timezone", Hint: "Standard five-field cron, with @daily / @hourly shorthand. Supports CRON_TZ.", Example: "termbelt cron '*/15 * * * *' --zone Asia/Dhaka", Local: true, Fields: []Field{{Key: "input", Label: "Cron expression", Placeholder: "*/15 * * * *"}, {Key: "zone", Label: "Timezone", Default: "Local", Help: "Local, UTC or IANA name"}, {Key: "count", Label: "Upcoming runs", Default: "8", Help: "1–50", Kind: IntField}, {Key: "from", Label: "Start from (optional)", Placeholder: "now · or 2026-01-01 09:00", Help: "Date or timestamp to preview from"}}},
	{ID: "regex", Name: "Regex playground", Category: "DEVELOPER", Description: "Test RE2 patterns and inspect captured groups", Hint: "Go / RE2 syntax. Paste sample text; use (?i) for case-insensitive matching.", Example: "termbelt regex '[a-z]+' 'hello 123 world'", Local: true, Fields: []Field{{Key: "pattern", Label: "Pattern", Placeholder: "(?P<word>[a-z]+)"}, {Key: "input", Label: "Sample text", Placeholder: "hello 123 world"}}},
	{ID: "cidr", Name: "Subnet calculator", Category: "NETWORK", Description: "IPv4 / IPv6 ranges, masks and address counts", Hint: "Runs locally. Enter a CIDR prefix; optionally check if an IP belongs to it.", Example: "termbelt cidr 192.168.1.0/24 --contains 192.168.1.42", Local: true, Fields: []Field{{Key: "input", Label: "CIDR prefix", Placeholder: "192.168.1.0/24"}, {Key: "contains", Label: "Check an IP (optional)", Placeholder: "192.168.1.42"}}},
}

func FindTool(id string) (Tool, bool) {
	for _, t := range Tools {
		if t.ID == id {
			return t, true
		}
	}
	return Tool{}, false
}

type Request struct {
	Tool    string
	Input   string
	Options map[string]string
	// InputReader streams CLI input for hashes without imposing the text-tool limit.
	InputReader io.Reader
}

func (r Request) Opt(key, fallback string) string {
	if v, ok := r.Options[key]; ok && v != "" {
		return v
	}
	return fallback
}
func (r Request) Bool(key string) bool { v := r.Opt(key, ""); return v == "true" || v == "1" }
func (r Request) Int(key string, fallback, lo, hi int) (int, error) {
	v, e := strconv.Atoi(r.Opt(key, strconv.Itoa(fallback)))
	if e != nil || v < lo || v > hi {
		return 0, fmt.Errorf("%s must be between %d and %d", key, lo, hi)
	}
	return v, nil
}

type Row struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type Section struct {
	Title string `json:"title"`
	Rows  []Row  `json:"rows,omitempty"`
	Text  string `json:"text,omitempty"`
}
type Table struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}
type Metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Unit  string `json:"unit,omitempty"`
}
type Result struct {
	Tool     string    `json:"tool"`
	Title    string    `json:"title"`
	Summary  string    `json:"summary,omitempty"`
	Metrics  []Metric  `json:"metrics,omitempty"`
	Sections []Section `json:"sections,omitempty"`
	Table    *Table    `json:"table,omitempty"`
	Output   string    `json:"output,omitempty"`
	// RawOutput replaces Output for --raw when the plain form differs, such as an unquoted JSON string.
	RawOutput  string   `json:"-"`
	Notes      []string `json:"notes,omitempty"`
	Data       any      `json:"data,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}
type Progress struct {
	Message  string
	Fraction float64
	Metrics  []Metric
}
type Emit func(Progress)

func report(emit Emit, p Progress) {
	if emit != nil {
		emit(p)
	}
}

type Engine struct {
	Client *http.Client
	Config Config
	RDAP   *Registry
}

func NewEngine(config Config) *Engine {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 64
	t.MaxIdleConnsPerHost = 6
	t.ResponseHeaderTimeout = time.Duration(config.TimeoutSeconds) * time.Second
	e := &Engine{Client: &http.Client{Transport: t, Timeout: time.Duration(config.TimeoutSeconds) * time.Second}, Config: config}
	e.RDAP = NewRegistry(e.Client)
	return e
}
func (e *Engine) Run(ctx context.Context, r Request, emit Emit) (result Result, err error) {
	start := time.Now()
	defer func() { result.Tool = r.Tool; result.DurationMS = time.Since(start).Milliseconds() }()
	if r.Options == nil {
		r.Options = map[string]string{}
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	switch r.Tool {
	case "speed":
		return e.Speed(ctx, r, emit)
	case "ip":
		return e.IP(ctx, r)
	case "domains":
		return e.Domains(ctx, r, emit)
	case "whois":
		return e.Whois(ctx, r)
	case "site":
		return e.Site(ctx, r, emit)
	case "dns":
		return e.DNS(ctx, r)
	case "tls":
		return e.TLS(ctx, r)
	case "http":
		return e.HTTP(ctx, r)
	case "ping":
		return e.Ping(ctx, r, emit)
	case "ports":
		return LocalPorts(ctx, r)
	case "hash":
		return hashToolContext(ctx, r)
	default:
		return RunLocal(r)
	}
}

// PrettyJSON indents without HTML escaping, so <, > and & stay readable.
func PrettyJSON(v any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimSuffix(b.String(), "\n")
}
func row(k string, v any) Row { return Row{k, fmt.Sprint(v)} }
func nonempty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
