package core

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"math/big"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/robfig/cron/v3"
)

func RunLocal(r Request) (Result, error) {
	switch r.Tool {
	case "json":
		return jsonTool(r)
	case "base64":
		return base64Tool(r)
	case "url":
		return urlTool(r)
	case "jwt":
		return jwtTool(r)
	case "hash":
		return hashTool(r)
	case "uuid":
		return uuidTool(r)
	case "password":
		return passwordTool(r)
	case "time":
		return timeTool(r)
	case "cron":
		return cronTool(r)
	case "regex":
		return regexTool(r)
	case "cidr":
		return cidrTool(r)
	default:
		return Result{}, fmt.Errorf("unknown tool %q", r.Tool)
	}
}
func decodeJSON(input string) (any, error) {
	d := json.NewDecoder(strings.NewReader(input))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected a single JSON value")
	}
	return v, nil
}
func jsonTool(r Request) (Result, error) {
	v, err := decodeJSON(r.Input)
	if err != nil {
		return Result{}, err
	}
	if path := r.Opt("path", ""); path != "" {
		for _, key := range strings.Split(path, ".") {
			switch x := v.(type) {
			case map[string]any:
				var ok bool
				v, ok = x[key]
				if !ok {
					return Result{}, fmt.Errorf("path key %q does not exist", key)
				}
			case []any:
				i, err := strconv.Atoi(key)
				if err != nil || i < 0 || i >= len(x) {
					return Result{}, fmt.Errorf("invalid array index %q", key)
				}
				v = x[i]
			default:
				return Result{}, fmt.Errorf("cannot traverse %q through a scalar", key)
			}
		}
	}
	var b []byte
	if r.Bool("minify") {
		b, err = json.Marshal(v)
	} else {
		b, err = json.MarshalIndent(v, "", "  ")
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Title: "JSON workbench", Summary: "Valid JSON · numeric precision preserved", Output: string(b), Data: v}, nil
}
func base64Tool(r Request) (Result, error) {
	mode := r.Opt("mode", "encode")
	if r.Bool("decode") {
		mode = "decode"
	}
	if mode != "encode" && mode != "decode" {
		return Result{}, fmt.Errorf("mode must be encode or decode")
	}
	if mode == "encode" {
		enc := base64.StdEncoding
		if r.Bool("url-safe") {
			enc = base64.RawURLEncoding
		}
		out := enc.EncodeToString([]byte(r.Input))
		return Result{Title: "Base64 encoded", Output: out, Data: map[string]any{"encoded": out, "input_bytes": len(r.Input)}}, nil
	}
	input := strings.Join(strings.Fields(r.Input), "")
	var b []byte
	var err error
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		b, err = enc.Strict().DecodeString(input)
		if err == nil {
			break
		}
	}
	if err != nil {
		return Result{}, fmt.Errorf("invalid Base64: %w", err)
	}
	out := string(b)
	notes := []string{}
	if !utf8.Valid(b) {
		out = hex.EncodeToString(b)
		notes = append(notes, "Decoded content is binary; displayed as hexadecimal.")
	}
	return Result{Title: "Base64 decoded", Output: out, Notes: notes, Data: map[string]any{"bytes": len(b), "hex": hex.EncodeToString(b)}}, nil
}
func urlTool(r Request) (Result, error) {
	mode := r.Opt("mode", "encode")
	if r.Bool("decode") {
		mode = "decode"
	}
	switch mode {
	case "encode":
		return Result{Title: "URL encoded", Output: strings.ReplaceAll(url.QueryEscape(r.Input), "+", "%20")}, nil
	case "decode":
		s, err := url.QueryUnescape(r.Input)
		if err != nil {
			return Result{}, err
		}
		return Result{Title: "URL decoded", Output: s}, nil
	case "inspect":
		u, err := url.Parse(r.Input)
		if err != nil {
			return Result{}, err
		}
		if u.Host == "" || u.Scheme == "" {
			return Result{}, fmt.Errorf("enter an absolute URL including its scheme")
		}
		rows := []Row{row("Scheme", u.Scheme), row("Host", u.Hostname()), row("Port", nonempty(u.Port(), "default")), row("Path", nonempty(u.Path, "/")), row("Fragment", u.Fragment)}
		for _, k := range sortedKeys(u.Query()) {
			rows = append(rows, row("Query · "+k, strings.Join(u.Query()[k], ", ")))
		}
		return Result{Title: "URL components", Sections: []Section{{Title: "COMPONENTS", Rows: rows}}, Data: map[string]any{"scheme": u.Scheme, "host": u.Hostname(), "port": u.Port(), "path": u.Path, "query": u.Query(), "fragment": u.Fragment}}, nil
	default:
		return Result{}, fmt.Errorf("mode must be encode, decode or inspect")
	}
}
func jwtTool(r Request) (Result, error) {
	parts := strings.Split(strings.TrimSpace(r.Input), ".")
	if len(parts) != 3 {
		return Result{}, fmt.Errorf("expected a JWT with three dot-separated segments")
	}
	var values [2]any
	for i := 0; i < 2; i++ {
		b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[i], "="))
		if err != nil {
			return Result{}, fmt.Errorf("invalid JWT segment %d: %w", i+1, err)
		}
		values[i], err = decodeJSON(string(b))
		if err != nil {
			return Result{}, err
		}
		if _, ok := values[i].(map[string]any); !ok {
			return Result{}, fmt.Errorf("JWT header and payload must be JSON objects")
		}
	}
	claims := values[1].(map[string]any)
	rows := []Row{}
	for _, key := range []string{"iat", "nbf", "exp"} {
		if val, ok := claims[key]; ok {
			num, ok := val.(json.Number)
			if !ok {
				rows = append(rows, row(key, "invalid NumericDate"))
				continue
			}
			f, err := num.Float64()
			if err != nil || f > 253402300799 || f < 0 {
				rows = append(rows, row(key, "invalid NumericDate"))
				continue
			}
			t := time.Unix(int64(f), 0)
			value := t.UTC().Format(time.RFC3339)
			if key == "exp" {
				if time.Now().After(t) {
					value += " · expired"
				} else {
					value += " · expires in " + time.Until(t).Round(time.Second).String()
				}
			}
			rows = append(rows, row(key, value))
		}
	}
	return Result{Title: "JWT inspector", Summary: "Decoded locally · signature NOT verified", Sections: []Section{{Title: "HEADER", Text: PrettyJSON(values[0])}, {Title: "CLAIMS", Text: PrettyJSON(values[1])}, {Title: "TIMESTAMPS", Rows: rows}}, Notes: []string{"Claims are untrusted. Decoding a token does not verify its signature or authenticity."}, Data: map[string]any{"header": values[0], "claims": claims, "signature_verified": false}}, nil
}
func hashTool(r Request) (Result, error) {
	return hashToolContext(context.Background(), r)
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}

func hashToolContext(ctx context.Context, r Request) (Result, error) {
	algo := strings.ToLower(r.Opt("algorithm", "sha256"))
	var h hash.Hash
	switch algo {
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	case "sha1":
		h = sha1.New()
	case "md5":
		h = md5.New()
	default:
		return Result{}, fmt.Errorf("algorithm must be sha256, sha512, sha1 or md5")
	}
	var source io.Reader = strings.NewReader(r.Input)
	label := "text"
	if r.InputReader != nil {
		source = r.InputReader
		label = "stdin"
	}
	if p := r.Opt("file", ""); p != "" {
		if r.Input != "" || r.InputReader != nil {
			return Result{}, fmt.Errorf("choose text, stdin or --file as the hash source")
		}
		info, err := os.Stat(p)
		if err != nil {
			return Result{}, err
		}
		if !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("--file requires a regular file; pipe streams into termbelt hash")
		}
		f, err := os.Open(p)
		if err != nil {
			return Result{}, err
		}
		defer f.Close()
		stopClosing := context.AfterFunc(ctx, func() { _ = f.Close() })
		defer stopClosing()
		info, err = f.Stat()
		if err != nil {
			return Result{}, err
		}
		if !info.Mode().IsRegular() {
			return Result{}, fmt.Errorf("--file requires a regular file")
		}
		source = f
		label = p
	}
	n, err := io.Copy(h, contextReader{ctx: ctx, source: source})
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err != nil {
		return Result{}, err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	notes := []string{}
	if algo == "md5" || algo == "sha1" {
		notes = append(notes, "Use MD5 / SHA-1 only for legacy checksums; use SHA-256 for security-sensitive integrity checks.")
	}
	return Result{Title: strings.ToUpper(algo) + " checksum", Summary: fmt.Sprintf("%s · %d bytes", label, n), Output: sum, Notes: notes, Data: map[string]any{"algorithm": algo, "hash": sum, "bytes": n}}, nil
}
func uuidTool(r Request) (Result, error) {
	n, err := r.Int("count", 1, 1, 100)
	if err != nil {
		return Result{}, err
	}
	ids := make([]string, n)
	for i := range ids {
		b := make([]byte, 16)
		if _, err = rand.Read(b); err != nil {
			return Result{}, err
		}
		b[6] = (b[6] & 15) | 64
		b[8] = (b[8] & 63) | 128
		ids[i] = fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
	}
	return Result{Title: "UUID v4", Summary: fmt.Sprintf("%d secure random identifier(s)", n), Output: strings.Join(ids, "\n"), Data: ids}, nil
}
func randomIndex(n int) (int, error) {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}
func passwordTool(r Request) (Result, error) {
	length, err := r.Int("length", 24, 12, 256)
	if err != nil {
		return Result{}, err
	}
	count, err := r.Int("count", 1, 1, 100)
	if err != nil {
		return Result{}, err
	}
	classes := []string{"abcdefghijkmnopqrstuvwxyz", "ABCDEFGHJKLMNPQRSTUVWXYZ", "23456789", "!@#$%^&*-_+=?"}
	if r.Bool("no-symbols") {
		classes = classes[:3]
	}
	alphabet := strings.Join(classes, "")
	passwords := make([]string, count)
	for i := range passwords {
		buf := make([]byte, length)
		for j := range buf {
			pool := alphabet
			if j < len(classes) {
				pool = classes[j]
			}
			idx, err := randomIndex(len(pool))
			if err != nil {
				return Result{}, err
			}
			buf[j] = pool[idx]
		}
		for j := len(buf) - 1; j > 0; j-- {
			k, err := randomIndex(j + 1)
			if err != nil {
				return Result{}, err
			}
			buf[j], buf[k] = buf[k], buf[j]
		}
		passwords[i] = string(buf)
	}
	return Result{Title: "Secure passwords", Summary: fmt.Sprintf("%d characters · crypto/rand · never saved", length), Output: strings.Join(passwords, "\n"), Data: passwords}, nil
}
func loadZone(name string) (*time.Location, error) {
	if name == "Local" || name == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone %q", name)
	}
	return loc, nil
}
func parseTime(input string, loc *time.Location) (time.Time, error) {
	return parseTimeUnit(input, loc, "auto")
}

var numericTimestamp = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)

func parseTimeUnit(input string, loc *time.Location, unit string) (time.Time, error) {
	input = strings.TrimSpace(input)
	unit = strings.ToLower(unit)
	if unit != "auto" && unit != "s" && unit != "ms" && unit != "us" && unit != "ns" {
		return time.Time{}, fmt.Errorf("unit must be auto, s, ms, us or ns")
	}
	if input == "" || input == "now" {
		return time.Now(), nil
	}
	if numericTimestamp.MatchString(input) {
		if len(input) > 40 {
			return time.Time{}, fmt.Errorf("timestamp is outside the supported date range")
		}
		if unit == "auto" {
			unit = "s"
			if !strings.Contains(input, ".") {
				digits := len(strings.TrimLeft(input, "+-0"))
				switch {
				case digits >= 19:
					unit = "ns"
				case digits >= 16:
					unit = "us"
				case digits >= 11:
					unit = "ms"
				}
			}
		}
		value, ok := new(big.Rat).SetString(input)
		if !ok {
			return time.Time{}, fmt.Errorf("invalid timestamp")
		}
		factor := map[string]int64{"s": 1e9, "ms": 1e6, "us": 1e3, "ns": 1}[unit]
		value.Mul(value, new(big.Rat).SetInt64(factor))
		nanos, fraction := new(big.Int), new(big.Int)
		nanos.QuoRem(value.Num(), value.Denom(), fraction)
		if fraction.Sign() != 0 {
			return time.Time{}, fmt.Errorf("timestamp precision must be at least one nanosecond")
		}
		seconds, remainder := new(big.Int), new(big.Int)
		seconds.QuoRem(nanos, big.NewInt(1e9), remainder)
		if !seconds.IsInt64() {
			return time.Time{}, fmt.Errorf("timestamp is outside the supported date range")
		}
		t := time.Unix(seconds.Int64(), remainder.Int64())
		if t.Year() < 1 || t.Year() > 9999 {
			return time.Time{}, fmt.Errorf("date must fall between years 1 and 9999")
		}
		return t, nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		t, err := time.ParseInLocation(layout, input, loc)
		if err == nil {
			if t.Year() < 1 || t.Year() > 9999 {
				return time.Time{}, fmt.Errorf("date must fall between years 1 and 9999")
			}
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("expected a Unix timestamp, RFC3339, or YYYY-MM-DD HH:MM:SS")
}
func timeTool(r Request) (Result, error) {
	loc, err := loadZone(r.Opt("zone", "Local"))
	if err != nil {
		return Result{}, err
	}
	t, err := parseTimeUnit(r.Input, loc, r.Opt("unit", "auto"))
	if err != nil {
		return Result{}, err
	}
	if t.Year() < 1 || t.Year() > 9999 {
		return Result{}, fmt.Errorf("date must fall between years 1 and 9999")
	}
	nanos := new(big.Int).Mul(big.NewInt(t.Unix()), big.NewInt(1e9))
	nanos.Add(nanos, big.NewInt(int64(t.Nanosecond())))
	data := map[string]any{"unix_seconds": t.Unix(), "unix_milliseconds": t.UnixMilli(), "unix_microseconds": t.UnixMicro(), "unix_nanoseconds": json.Number(nanos.String()), "utc": t.UTC().Format(time.RFC3339Nano), "local": t.In(loc).Format(time.RFC3339Nano), "timezone": loc.String()}
	return Result{Title: "Timestamp converter", Sections: []Section{{Title: "TIME", Rows: []Row{row("Unix seconds", t.Unix()), row("Unix milliseconds", t.UnixMilli()), row("Unix microseconds", t.UnixMicro()), row("Unix nanoseconds", nanos), row("UTC", t.UTC().Format(time.RFC3339Nano)), row(loc.String(), t.In(loc).Format("Mon, 02 Jan 2006 · 15:04:05.999999999 MST"))}}}, Notes: []string{"Auto-detection uses timestamp magnitude; use --unit s, ms, us or ns for small or historical values."}, Data: data}, nil
}
func cronTool(r Request) (Result, error) {
	loc, err := loadZone(r.Opt("zone", "Local"))
	if err != nil {
		return Result{}, err
	}
	count, err := r.Int("count", 8, 1, 50)
	if err != nil {
		return Result{}, err
	}
	expr := strings.TrimSpace(r.Input)
	if expr == "" {
		return Result{}, fmt.Errorf("enter a five-field cron expression")
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return Result{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	current := time.Now().In(loc)
	if r.Opt("from", "") != "" {
		current, err = parseTime(r.Opt("from", ""), loc)
		if err != nil {
			return Result{}, err
		}
		current = current.In(loc)
	}
	runs := []string{}
	table := &Table{Headers: []string{"#", "NEXT RUN", "FROM PREVIOUS"}}
	for i := 0; i < count; i++ {
		next := schedule.Next(current)
		if next.IsZero() {
			return Result{}, fmt.Errorf("schedule has no matching dates in the next five years")
		}
		runs = append(runs, next.In(loc).Format(time.RFC3339))
		table.Rows = append(table.Rows, []string{strconv.Itoa(i + 1), next.In(loc).Format("Mon 02 Jan 2006 · 15:04 MST"), next.Sub(current).String()})
		current = next
	}
	return Result{Title: "Cron explorer", Summary: expr + " · " + loc.String(), Table: table, Data: map[string]any{"expression": expr, "timezone": loc.String(), "next_runs": runs}}, nil
}
func regexTool(r Request) (Result, error) {
	pattern := r.Opt("pattern", "")
	if pattern == "" {
		return Result{}, fmt.Errorf("a regex pattern is required")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Result{}, err
	}
	matches := re.FindAllStringSubmatchIndex(r.Input, 1001)
	truncated := len(matches) > 1000
	if truncated {
		matches = matches[:1000]
	}
	table := &Table{Headers: []string{"#", "MATCH", "BYTE RANGE", "GROUPS"}}
	data := []map[string]any{}
	for i, m := range matches {
		groups := map[string]string{}
		for j := 1; j < len(m)/2; j++ {
			if m[j*2] < 0 {
				continue
			}
			name := re.SubexpNames()[j]
			if name == "" {
				name = strconv.Itoa(j)
			}
			groups[name] = r.Input[m[j*2]:m[j*2+1]]
		}
		value := r.Input[m[0]:m[1]]
		table.Rows = append(table.Rows, []string{strconv.Itoa(i + 1), value, fmt.Sprintf("%d:%d", m[0], m[1]), PrettyJSON(groups)})
		data = append(data, map[string]any{"match": value, "start": m[0], "end": m[1], "groups": groups})
	}
	notes := []string{"Go / RE2 syntax; backreferences and lookaround are not supported."}
	if truncated {
		notes = append(notes, "Output capped at 1,000 matches.")
	}
	return Result{Title: "Regex playground", Summary: fmt.Sprintf("%d matches", len(matches)), Table: table, Notes: notes, Data: data}, nil
}
func cidrTool(r Request) (Result, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(r.Input))
	if err != nil {
		return Result{}, fmt.Errorf("invalid CIDR prefix: %w", err)
	}
	p = p.Masked()
	addr := p.Addr()
	bits := addr.BitLen() - p.Bits()
	count := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	raw := addr.AsSlice()
	n := new(big.Int).SetBytes(raw)
	lastN := new(big.Int).Add(n, new(big.Int).Sub(new(big.Int).Set(count), big.NewInt(1)))
	lastBytes := lastN.FillBytes(make([]byte, len(raw)))
	last, _ := netip.AddrFromSlice(lastBytes)
	rows := []Row{row("Network", p.String()), row("IP version", fmt.Sprintf("IPv%d", map[bool]int{true: 4, false: 6}[addr.Is4()])), row("First address", addr.String()), row("Last address", last.String()), row("Addresses", count.String())}
	data := map[string]any{"network": p.String(), "first": addr.String(), "last": last.String(), "addresses": count.String()}
	if addr.Is4() {
		mask := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 32), count)
		maskBytes := mask.FillBytes(make([]byte, 4))
		maskAddr, _ := netip.AddrFromSlice(maskBytes)
		usable := new(big.Int).Set(count)
		if bits > 1 {
			usable.Sub(usable, big.NewInt(2))
		}
		rows = append(rows, row("Netmask", maskAddr.String()), row("Usable hosts", usable.String()), row("Broadcast", last.String()))
		if bits <= 1 {
			rows[len(rows)-1] = row("Broadcast", "none (/31 or /32)")
		}
	}
	if input := r.Opt("contains", ""); input != "" {
		ip, err := netip.ParseAddr(input)
		if err != nil {
			return Result{}, err
		}
		contains := p.Contains(ip)
		rows = append(rows, row("Contains "+input, contains))
		data["contains"] = contains
	}
	return Result{Title: "Subnet calculator", Sections: []Section{{Title: "ADDRESS SPACE", Rows: rows}}, Data: data}, nil
}

// Keep byte-oriented decode helpers small and independently testable.
func compactJSON(b []byte) ([]byte, error) {
	var out bytes.Buffer
	err := json.Compact(&out, b)
	return out.Bytes(), err
}
