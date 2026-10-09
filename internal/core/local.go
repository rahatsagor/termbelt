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
	input := strings.TrimSpace(r.Input)
	if _, err := decodeJSON(input); err != nil {
		return Result{}, err
	}
	value := json.RawMessage(input)
	summary := "Valid JSON · key order and numeric precision preserved"
	if path := strings.TrimSpace(r.Opt("path", "")); path != "" {
		segments, err := parseJSONPath(path)
		if err != nil {
			return Result{}, err
		}
		for _, segment := range segments {
			if value, err = jsonChild(value, segment); err != nil {
				return Result{}, err
			}
		}
		summary = "Value at " + path
	}
	var formatted bytes.Buffer
	var err error
	if r.Bool("minify") {
		err = json.Compact(&formatted, value)
	} else {
		err = json.Indent(&formatted, value, "", "  ")
	}
	if err != nil {
		return Result{}, err
	}
	compact, err := compactJSON(value)
	if err != nil {
		return Result{}, err
	}
	result := Result{Title: "JSON workbench", Summary: summary, Output: formatted.String(), Data: json.RawMessage(compact)}
	var text string
	if len(compact) > 0 && compact[0] == '"' && json.Unmarshal(compact, &text) == nil {
		result.RawOutput = text
	}
	return result, nil
}

// parseJSONPath accepts dotted paths (users.0.name) and bracket segments
// (users[0]["display.name"]) for keys that contain dots or brackets.
func parseJSONPath(path string) ([]string, error) {
	segments := []string{}
	current := strings.Builder{}
	flush := func() {
		if current.Len() > 0 {
			segments = append(segments, current.String())
			current.Reset()
		}
	}
	for i := 0; i < len(path); i++ {
		switch c := path[i]; c {
		case '.':
			flush()
		case '[':
			flush()
			end := strings.IndexByte(path[i:], ']')
			if i+1 < len(path) && (path[i+1] == '"' || path[i+1] == '\'') {
				quote := path[i+1]
				closing := -1
				for j := i + 2; j < len(path); j++ {
					if path[j] == '\\' {
						j++
						continue
					}
					if path[j] == quote {
						closing = j
						break
					}
				}
				if closing < 0 || closing+1 >= len(path) || path[closing+1] != ']' {
					return nil, fmt.Errorf("unterminated quoted key in path")
				}
				key := path[i+2 : closing]
				if quote == '"' {
					if err := json.Unmarshal([]byte(path[i+1:closing+1]), &key); err != nil {
						return nil, fmt.Errorf("invalid quoted key in path")
					}
				}
				segments = append(segments, key)
				i = closing + 1
				continue
			}
			if end < 0 {
				return nil, fmt.Errorf("unterminated [ in path")
			}
			segments = append(segments, path[i+1:i+end])
			i += end
		default:
			current.WriteByte(c)
		}
	}
	flush()
	if len(segments) == 0 {
		return nil, fmt.Errorf("empty JSON path")
	}
	return segments, nil
}

func jsonChild(value json.RawMessage, key string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("cannot traverse %q through an empty value", key)
	}
	switch trimmed[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return nil, err
		}
		child, ok := object[key]
		if !ok {
			return nil, fmt.Errorf("path key %q does not exist", key)
		}
		return child, nil
	case '[':
		var array []json.RawMessage
		if err := json.Unmarshal(trimmed, &array); err != nil {
			return nil, err
		}
		i, err := strconv.Atoi(key)
		if err == nil && i < 0 {
			i += len(array)
		}
		if err != nil || i < 0 || i >= len(array) {
			return nil, fmt.Errorf("invalid array index %q (length %d)", key, len(array))
		}
		return array[i], nil
	default:
		return nil, fmt.Errorf("cannot traverse %q through a scalar", key)
	}
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
	status := "active"
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
			switch {
			case key == "exp" && time.Now().After(t):
				value += " · expired " + time.Since(t).Round(time.Second).String() + " ago"
				status = "expired"
			case key == "exp":
				value += " · expires in " + time.Until(t).Round(time.Second).String()
			case key == "nbf" && time.Now().Before(t):
				value += " · not valid yet"
				status = "not yet valid"
			case key == "iat" && time.Now().Before(t):
				value += " · issued in the future"
			}
			rows = append(rows, row(key, value))
		}
	}
	if _, ok := claims["exp"]; !ok {
		status = "no expiry"
	}
	header := values[0].(map[string]any)
	notes := []string{"Claims are untrusted. Decoding a token does not verify its signature or authenticity."}
	if alg, _ := header["alg"].(string); strings.EqualFold(alg, "none") {
		notes = append(notes, "The header declares alg \"none\": this token is unsigned.")
	}
	return Result{Title: "JWT inspector", Summary: "Decoded locally · " + status + " · signature NOT verified", Sections: []Section{{Title: "HEADER", Text: PrettyJSON(values[0])}, {Title: "CLAIMS", Text: PrettyJSON(values[1])}, {Title: "TIMESTAMPS", Rows: rows}}, Notes: notes, Data: map[string]any{"header": values[0], "claims": claims, "status": status, "signature_verified": false}}, nil
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
	algo := strings.ToLower(strings.TrimSpace(r.Opt("algorithm", "sha256")))
	constructors := map[string]func() hash.Hash{"sha256": sha256.New, "sha512": sha512.New, "sha384": sha512.New384, "sha224": sha256.New224, "sha1": sha1.New, "md5": md5.New}
	names := []string{algo}
	if algo == "all" {
		names = []string{"sha256", "sha512", "sha384", "sha224", "sha1", "md5"}
	}
	hashes := make([]hash.Hash, len(names))
	writers := make([]io.Writer, len(names))
	for i, name := range names {
		constructor, ok := constructors[name]
		if !ok {
			return Result{}, fmt.Errorf("algorithm must be sha256, sha512, sha384, sha224, sha1, md5 or all")
		}
		hashes[i] = constructor()
		writers[i] = hashes[i]
	}
	h := io.MultiWriter(writers...)
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
	notes := []string{}
	if algo == "md5" || algo == "sha1" {
		notes = append(notes, "Use MD5 / SHA-1 only for legacy checksums; use SHA-256 for security-sensitive integrity checks.")
	}
	if len(names) == 1 {
		sum := hex.EncodeToString(hashes[0].Sum(nil))
		if expected := strings.ToLower(strings.TrimSpace(r.Opt("verify", ""))); expected != "" {
			return hashVerification(algo, label, n, sum, expected, notes)
		}
		return Result{Title: strings.ToUpper(algo) + " checksum", Summary: fmt.Sprintf("%s · %d bytes", label, n), Output: sum, Notes: notes, Data: map[string]any{"algorithm": algo, "hash": sum, "bytes": n}}, nil
	}
	rows := []Row{}
	sums := map[string]string{}
	lines := []string{}
	for i, name := range names {
		sum := hex.EncodeToString(hashes[i].Sum(nil))
		sums[name] = sum
		rows = append(rows, row(strings.ToUpper(name), sum))
		lines = append(lines, fmt.Sprintf("%-6s  %s", name, sum))
	}
	if expected := strings.ToLower(strings.TrimSpace(r.Opt("verify", ""))); expected != "" {
		for _, name := range names {
			if sums[name] == expected {
				return hashVerification(name, label, n, sums[name], expected, notes)
			}
		}
		return hashVerification("any", label, n, "", expected, notes)
	}
	return Result{Title: "Checksums", Summary: fmt.Sprintf("%s · %d bytes", label, n), Sections: []Section{{Title: "DIGESTS", Rows: rows}}, RawOutput: strings.Join(lines, "\n"), Notes: []string{"MD5 and SHA-1 are suitable only for legacy checksums."}, Data: map[string]any{"algorithm": "all", "hashes": sums, "bytes": n}}, nil
}

func hashVerification(algo, label string, n int64, sum, expected string, notes []string) (Result, error) {
	if sum != expected {
		return Result{}, fmt.Errorf("checksum mismatch for %s: expected %s, got %s", label, expected, nonempty(sum, "no matching algorithm"))
	}
	return Result{Title: strings.ToUpper(algo) + " checksum verified", Summary: fmt.Sprintf("%s · %d bytes · matches", label, n), Output: sum, Notes: notes, Data: map[string]any{"algorithm": algo, "hash": sum, "bytes": n, "verified": true}}, nil
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
	input := strings.TrimSpace(r.Input)
	if ip, err := netip.ParseAddr(input); err == nil && !strings.Contains(input, "/") {
		input = netip.PrefixFrom(ip, ip.BitLen()).String()
	}
	p, err := netip.ParsePrefix(input)
	if err != nil {
		return Result{}, fmt.Errorf("invalid CIDR prefix (expected an address/length such as 10.0.0.0/8): %w", err)
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
		wildcard := make([]byte, len(maskBytes))
		for i, b := range maskBytes {
			wildcard[i] = ^b
		}
		wildcardAddr, _ := netip.AddrFromSlice(wildcard)
		firstHost, lastHost := addr, last
		if bits > 1 {
			firstHost, lastHost = addr.Next(), last.Prev()
		}
		rows = append(rows, row("Netmask", maskAddr.String()), row("Wildcard", wildcardAddr.String()), row("Usable hosts", usable.String()), row("Host range", firstHost.String()+" – "+lastHost.String()), row("Broadcast", last.String()))
		data["netmask"], data["wildcard"], data["usable_hosts"] = maskAddr.String(), wildcardAddr.String(), usable.String()
		data["first_host"], data["last_host"] = firstHost.String(), lastHost.String()
		if bits <= 1 {
			rows[len(rows)-1] = row("Broadcast", "none (/31 or /32)")
		}
	}
	if input := strings.TrimSpace(r.Opt("contains", "")); input != "" {
		ip, err := netip.ParseAddr(input)
		if err != nil {
			return Result{}, fmt.Errorf("invalid IP for --contains: %w", err)
		}
		contains := p.Contains(ip)
		rows = append(rows, row("Contains "+input, contains))
		data["contains"] = contains
	}
	return Result{Title: "Subnet calculator", Sections: []Section{{Title: "ADDRESS SPACE", Rows: rows}}, Data: data}, nil
}

func compactJSON(b []byte) ([]byte, error) {
	var out bytes.Buffer
	err := json.Compact(&out, b)
	return out.Bytes(), err
}
