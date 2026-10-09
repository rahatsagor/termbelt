package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

func localRequest(tool, input string, options map[string]string) Request {
	return Request{Tool: tool, Input: input, Options: options}
}

func TestJSONToolPreservesLargeIntegerAndExtractsNestedPath(t *testing.T) {
	const input = `{"items":[{"id":9007199254740993}],"ok":true}`
	result, err := RunLocal(localRequest("json", input, map[string]string{"path": "items.0.id"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != `9007199254740993` {
		t.Fatalf("large integer lost precision: got %q", result.Output)
	}
	n, ok := result.Data.(json.RawMessage)
	if !ok || string(n) != "9007199254740993" {
		t.Fatalf("path data = %#v, want precise raw JSON", result.Data)
	}
}

func TestJSONToolKeepsKeyOrderHTMLAndBracketPaths(t *testing.T) {
	const input = `{"z":1,"a":"<b>&","nested":{"display.name":"Ada","list":[1,2,3]}}`
	result, err := RunLocal(localRequest("json", input, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Output, "{\n  \"z\": 1,\n  \"a\": \"<b>&\"") {
		t.Fatalf("formatted output reordered or escaped: %s", result.Output)
	}
	minified, err := RunLocal(localRequest("json", input, map[string]string{"minify": "true"}))
	if err != nil || minified.Output != input {
		t.Fatalf("minified = %q, %v", minified.Output, err)
	}
	for path, want := range map[string]string{`nested["display.name"]`: `"Ada"`, `nested.list[-1]`: `3`, `nested.list.0`: `1`} {
		got, err := RunLocal(localRequest("json", input, map[string]string{"path": path}))
		if err != nil || got.Output != want {
			t.Errorf("path %s = %q, %v; want %s", path, got.Output, err, want)
		}
	}
	text, _ := RunLocal(localRequest("json", input, map[string]string{"path": `nested["display.name"]`}))
	if text.RawOutput != "Ada" {
		t.Fatalf("raw string = %q", text.RawOutput)
	}
	if out := PrettyJSON(map[string]string{"html": "<a>&"}); !strings.Contains(out, `"<a>&"`) {
		t.Fatalf("PrettyJSON escaped HTML: %s", out)
	}
}

func TestHashAllAlgorithmsAndVerification(t *testing.T) {
	all, err := RunLocal(localRequest("hash", "abc", map[string]string{"algorithm": "all"}))
	if err != nil {
		t.Fatal(err)
	}
	sums := all.Data.(map[string]any)["hashes"].(map[string]string)
	if sums["sha384"] != "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7" || len(sums) != 6 {
		t.Fatalf("sums = %#v", sums)
	}
	ok, err := RunLocal(localRequest("hash", "abc", map[string]string{"verify": "BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD"}))
	if err != nil || ok.Data.(map[string]any)["verified"] != true {
		t.Fatalf("verification = %#v, %v", ok.Data, err)
	}
	if _, err := RunLocal(localRequest("hash", "abc", map[string]string{"verify": "00"})); err == nil {
		t.Fatal("mismatched checksum was accepted")
	}
}

func TestJSONToolRejectsMalformedAndTrailingValues(t *testing.T) {
	for _, input := range []string{`{"broken":`, `{} {}`, `{} trailing`} {
		t.Run(input, func(t *testing.T) {
			if _, err := RunLocal(localRequest("json", input, nil)); err == nil {
				t.Fatalf("accepted invalid or trailing JSON %q", input)
			}
		})
	}
}

func TestBase64EncodeDecodeStandardAndURLSafe(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		url  bool
	}{
		{name: "standard", text: "hello, world"},
		{name: "URL-safe", text: "ÿÿÿ", url: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := map[string]string{}
			if tc.url {
				options["url-safe"] = "true"
			}
			encoded, err := RunLocal(localRequest("base64", tc.text, options))
			if err != nil {
				t.Fatal(err)
			}
			if tc.url && !strings.ContainsAny(encoded.Output, "-_") {
				t.Fatalf("URL-safe encoding did not exercise its alternate alphabet: %q", encoded.Output)
			}
			decoded, err := RunLocal(localRequest("base64", encoded.Output, map[string]string{"mode": "decode"}))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Output != tc.text {
				t.Fatalf("roundtrip got %q, want %q", decoded.Output, tc.text)
			}
		})
	}
}

func TestBase64DecodeAcceptsRawURLAndRejectsMalformed(t *testing.T) {
	encoded := base64.RawURLEncoding.EncodeToString([]byte("binary-ish ?xfb?"))
	got, err := RunLocal(localRequest("base64", encoded, map[string]string{"mode": "decode"}))
	if err != nil || got.Output != "binary-ish ?xfb?" {
		t.Fatalf("raw URL decode = %q, %v", got.Output, err)
	}
	if _, err := RunLocal(localRequest("base64", "a===", map[string]string{"mode": "decode"})); err == nil {
		t.Fatal("malformed Base64 was accepted")
	}
}

func TestURLCodecRoundTripReservedUnicodeAndSpaces(t *testing.T) {
	input := "https://example.test/a path?q=fish & chips&emoji=☃#frag"
	encoded, err := RunLocal(localRequest("url", input, nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encoded.Output, "+") {
		t.Fatalf("spaces should use %%20, got %q", encoded.Output)
	}
	decoded, err := RunLocal(localRequest("url", encoded.Output, map[string]string{"mode": "decode"}))
	if err != nil || decoded.Output != input {
		t.Fatalf("URL roundtrip = %q, %v", decoded.Output, err)
	}
}

func TestJWTDecodesClaimsWithoutClaimingSignatureVerification(t *testing.T) {
	segment := func(v string) string { return base64.RawURLEncoding.EncodeToString([]byte(v)) }
	token := segment(`{"alg":"none","typ":"JWT"}`) + "." + segment(`{"sub":"user-7","role":"reader"}`) + ".unsigned"
	result, err := RunLocal(localRequest("jwt", token, nil))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := result.Data.(map[string]any)
	if !ok || data["signature_verified"] != false {
		t.Fatalf("signature status = %#v, want false", data)
	}
	claims := data["claims"].(map[string]any)
	if claims["sub"] != "user-7" || claims["role"] != "reader" {
		t.Fatalf("claims = %#v", claims)
	}
}

func TestHashKnownChecksumsAndFileStreaming(t *testing.T) {
	known := map[string]string{
		"md5":    "900150983cd24fb0d6963f7d28e17f72",
		"sha1":   "a9993e364706816aba3e25717850c26c9cd0d89d",
		"sha256": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"sha512": "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
	}
	for algo, want := range known {
		t.Run(algo, func(t *testing.T) {
			result, err := RunLocal(localRequest("hash", "abc", map[string]string{"algorithm": algo}))
			if err != nil || result.Output != want {
				t.Fatalf("hash = %q, %v; want %s", result.Output, err, want)
			}
		})
	}
	path := t.TempDir() + "/payload.bin"
	content := strings.Repeat("stream chunk\x00", 8192)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	want, err := RunLocal(localRequest("hash", "", map[string]string{"algorithm": "sha256", "file": path}))
	if err != nil {
		t.Fatal(err)
	}
	textHash, _ := RunLocal(localRequest("hash", content, map[string]string{"algorithm": "sha256"}))
	if want.Output != textHash.Output {
		t.Fatalf("file hash differs from content hash: %s != %s", want.Output, textHash.Output)
	}
}

func TestUUIDsAreV4VariantAndUnique(t *testing.T) {
	result, err := RunLocal(localRequest("uuid", "", map[string]string{"count": "64"}))
	if err != nil {
		t.Fatal(err)
	}
	ids := result.Data.([]string)
	if len(ids) != 64 {
		t.Fatalf("generated %d UUIDs", len(ids))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		compact := strings.ReplaceAll(id, "-", "")
		if len(compact) != 32 || compact[12] != '4' || !strings.ContainsRune("89ab", rune(compact[16])) {
			t.Fatalf("not a UUID v4 with RFC variant: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate UUID %q", id)
		}
		seen[id] = true
	}
}

func TestPasswordClassesLengthAndOptionValidation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		options    map[string]string
		wantSymbol bool
	}{
		{name: "all classes", options: map[string]string{"length": "48"}, wantSymbol: true},
		{name: "no symbols", options: map[string]string{"length": "32", "no-symbols": "true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := RunLocal(localRequest("password", "", tc.options))
			if err != nil {
				t.Fatal(err)
			}
			password := result.Data.([]string)[0]
			if len(password) != mustAtoi(t, tc.options["length"]) {
				t.Fatalf("length %d, password %q", len(password), password)
			}
			for _, class := range []string{"abcdefghijkmnopqrstuvwxyz", "ABCDEFGHJKLMNPQRSTUVWXYZ", "23456789"} {
				if !strings.ContainsAny(password, class) {
					t.Fatalf("missing required class from %q", password)
				}
			}
			hasSymbol := strings.ContainsAny(password, "!@#$%^&*-_+=?")
			if hasSymbol != tc.wantSymbol {
				t.Fatalf("symbol presence %t, want %t", hasSymbol, tc.wantSymbol)
			}
		})
	}
	for _, options := range []map[string]string{{"length": "11"}, {"length": "257"}, {"count": "0"}, {"count": "101"}, {"length": "nope"}} {
		if _, err := RunLocal(localRequest("password", "", options)); err == nil {
			t.Errorf("invalid options accepted: %#v", options)
		}
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTimeParsesSecondsMillisecondsAndTimezone(t *testing.T) {
	seconds, err := RunLocal(localRequest("time", "1700000000", map[string]string{"zone": "Asia/Dhaka"}))
	if err != nil {
		t.Fatal(err)
	}
	secData := seconds.Data.(map[string]any)
	if secData["unix_seconds"] != int64(1700000000) || secData["timezone"] != "Asia/Dhaka" {
		t.Fatalf("seconds data = %#v", secData)
	}
	millis, err := RunLocal(localRequest("time", "1700000000123", map[string]string{"zone": "Asia/Dhaka"}))
	if err != nil {
		t.Fatal(err)
	}
	msData := millis.Data.(map[string]any)
	if msData["unix_milliseconds"] != int64(1700000000123) {
		t.Fatalf("millisecond data = %#v", msData)
	}
	if _, err := RunLocal(localRequest("time", "2025-03-01 12:00:00", map[string]string{"zone": "Asia/Dhaka"})); err != nil {
		t.Fatalf("local wall time in IANA zone rejected: %v", err)
	}
	if _, err := RunLocal(localRequest("time", "1700000000", map[string]string{"zone": "Mars/Olympus"})); err == nil {
		t.Fatal("unknown timezone accepted")
	}
}

func TestCronFromProducesDeterministicRuns(t *testing.T) {
	options := map[string]string{"from": "2025-01-01T00:00:00Z", "zone": "UTC", "count": "3"}
	first, err := RunLocal(localRequest("cron", "0 * * * *", options))
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunLocal(localRequest("cron", "0 * * * *", options))
	if err != nil {
		t.Fatal(err)
	}
	a := first.Data.(map[string]any)["next_runs"].([]string)
	b := second.Data.(map[string]any)["next_runs"].([]string)
	want := []string{"2025-01-01T01:00:00Z", "2025-01-01T02:00:00Z", "2025-01-01T03:00:00Z"}
	if fmt.Sprint(a) != fmt.Sprint(want) || fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("runs = %v and %v, want %v", a, b, want)
	}
}

func TestRegexNamedCaptureAndInvalidPattern(t *testing.T) {
	result, err := RunLocal(localRequest("regex", "user=42 user=7", map[string]string{"pattern": `user=(?P<id>\d+)`}))
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.([]map[string]any)
	if len(data) != 2 || data[0]["groups"].(map[string]string)["id"] != "42" || data[1]["groups"].(map[string]string)["id"] != "7" {
		t.Fatalf("matches = %#v", data)
	}
	if _, err := RunLocal(localRequest("regex", "anything", map[string]string{"pattern": "("})); err == nil {
		t.Fatal("invalid regex accepted")
	}
}

func TestCIDRBoundariesAndContains(t *testing.T) {
	for _, tc := range []struct {
		prefix    string
		addresses string
		last      string
		contains  string
		want      bool
	}{
		{"192.0.2.129/24", "256", "192.0.2.255", "192.0.2.200", true},
		{"192.0.2.4/31", "2", "192.0.2.5", "192.0.2.5", true},
		{"192.0.2.7/32", "1", "192.0.2.7", "192.0.2.8", false},
		{"2001:db8:abcd:1::1234/64", "18446744073709551616", "2001:db8:abcd:1:ffff:ffff:ffff:ffff", "2001:db8:abcd:1::1", true},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			result, err := RunLocal(localRequest("cidr", tc.prefix, map[string]string{"contains": tc.contains}))
			if err != nil {
				t.Fatal(err)
			}
			data := result.Data.(map[string]any)
			if data["addresses"] != tc.addresses || data["last"] != tc.last || data["contains"] != tc.want {
				t.Fatalf("CIDR data = %#v", data)
			}
		})
	}
}
