package core

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRegistrableAcceptsPastedURLForms(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"example.com/", "example.com"},
		{"example.com:443", "example.com"},
		{"https://www.example.co.uk:8443/x?y=1", "example.co.uk"},
	} {
		got, err := registrable(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("registrable(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
	if _, err := domainASCII("my idea"); err == nil || !strings.Contains(err.Error(), "invalid domain name") {
		t.Errorf("domainASCII(my idea) error = %v; want invalid domain name", err)
	}
	if got, err := domainASCII("myidea"); err != nil || got != "myidea" {
		t.Errorf("domainASCII(myidea) = %q, %v", got, err)
	}
}

func TestLoadZoneAcceptsLocalAndUTCCaseInsensitively(t *testing.T) {
	for _, tc := range []struct {
		name string
		want *time.Location
	}{
		{"local", time.Local},
		{"utc", time.UTC},
		{" UTC ", time.UTC},
	} {
		got, err := loadZone(tc.name)
		if err != nil || got != tc.want {
			t.Errorf("loadZone(%q) = %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
	if _, err := loadZone("Asia/Dhaka"); err != nil {
		t.Errorf("loadZone(Asia/Dhaka): %v", err)
	}
	if _, err := loadZone("Nope/Zone"); err == nil {
		t.Error("loadZone(Nope/Zone) accepted an unknown zone")
	}
}

func TestIPRowsShowLocalClassificationAndOmitMissingGeoData(t *testing.T) {
	rows := ipRows(IPData{IP: "10.0.0.1", Source: "local classification", Organization: "Private network"})
	want := []Row{row("IP address", "10.0.0.1"), row("Classification", "Private network"), row("Source", "local classification")}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Fatalf("local rows = %#v, want %#v", rows, want)
	}
	geo := ipRows(IPData{IP: "1.2.3.4", City: "Dhaka", Country: "Bangladesh", Source: "provider"})
	if geo[1] != row("Location", "Dhaka, Bangladesh") || geo[6] != row("Coordinates", "not provided") {
		t.Fatalf("geo rows = %#v", geo)
	}
}

func TestDNSFailureSummaryCollapsesIdenticalReasons(t *testing.T) {
	if got := dnsFailureSummary([]string{"A: DNS NXDOMAIN", "AAAA: DNS NXDOMAIN"}); got != "DNS NXDOMAIN for all 2 record types" {
		t.Fatalf("identical reasons = %q", got)
	}
	if got := dnsFailureSummary([]string{"A: DNS NXDOMAIN", "AAAA: connection refused"}); got != "A: DNS NXDOMAIN; AAAA: connection refused" {
		t.Fatalf("differing reasons = %q", got)
	}
	if got := dnsFailureSummary([]string{"A: DNS NXDOMAIN"}); got != "A: DNS NXDOMAIN" {
		t.Fatalf("single note = %q", got)
	}
}

func TestURLInspectMasksPasswordAndNotesUndecodableQuery(t *testing.T) {
	result, err := RunLocal(Request{Tool: "url", Input: "https://user:pw@example.com:8443/a?x=1&x=2&bad=%zz#f", Options: map[string]string{"mode": "inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, r := range result.Sections[0].Rows {
		values[r.Label] = r.Value
	}
	if values["User"] != "user" || values["Password"] != "********" || values["Query · x"] != "1, 2" {
		t.Fatalf("component rows = %#v", result.Sections[0].Rows)
	}
	decoded := 0
	for _, note := range result.Notes {
		if strings.Contains(note, "could not be decoded") {
			decoded++
		}
	}
	if decoded != 1 {
		t.Fatalf("notes = %v", result.Notes)
	}
	data := result.Data.(map[string]any)
	if data["user"] != "user" || data["password"] != "********" {
		t.Fatalf("data = %#v", data)
	}
	clean, err := RunLocal(Request{Tool: "url", Input: "https://example.com/?a=1", Options: map[string]string{"mode": "inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range clean.Sections[0].Rows {
		if r.Label == "User" || r.Label == "Password" {
			t.Fatalf("unexpected credential row %#v", r)
		}
	}
	if len(clean.Notes) != 0 {
		t.Fatalf("unexpected notes %v", clean.Notes)
	}
	if _, ok := clean.Data.(map[string]any)["user"]; ok {
		t.Fatal("user key present without userinfo")
	}
	for _, tc := range []struct {
		input       string
		hasPassword bool
	}{
		{"https://user@example.com/", false},
		{"https://user:@example.com/", true},
	} {
		result, err := RunLocal(Request{Tool: "url", Input: tc.input, Options: map[string]string{"mode": "inspect"}})
		if err != nil {
			t.Fatal(err)
		}
		data := result.Data.(map[string]any)
		masked, ok := data["password"]
		if data["user"] != "user" || ok != tc.hasPassword || (ok && masked != "********") {
			t.Errorf("userinfo for %q = %#v", tc.input, data)
		}
	}
}

func TestCIDRContainsUnmapsIPv4MappedIPv6(t *testing.T) {
	for _, tc := range []struct {
		prefix, contains string
		want             bool
	}{
		{"10.0.0.0/8", "::ffff:10.1.1.1", true},
		{"10.0.0.0/8", "::ffff:11.1.1.1", false},
		{"2001:db8::/32", "10.1.1.1", false},
	} {
		result, err := RunLocal(Request{Tool: "cidr", Input: tc.prefix, Options: map[string]string{"contains": tc.contains}})
		if err != nil {
			t.Fatal(err)
		}
		if got := result.Data.(map[string]any)["contains"]; got != tc.want {
			t.Fatalf("%s contains %s: got %#v, want %v", tc.prefix, tc.contains, got, tc.want)
		}
	}
}

func TestJWTErrorsOnEncryptedAndRoundsLongExpirations(t *testing.T) {
	if _, err := RunLocal(Request{Tool: "jwt", Input: "a.b.c.d.e"}); err == nil || !strings.Contains(err.Error(), "JWE") {
		t.Fatalf("five-segment token error = %v, want a JWE explanation", err)
	}
	if got := roughDuration(90 * time.Second); got != "1m30s" {
		t.Fatalf("roughDuration(90s) = %q, want 1m30s", got)
	}
	if got := roughDuration(72 * time.Hour); got != "3 days" {
		t.Fatalf("roughDuration(72h) = %q, want 3 days", got)
	}
}

func TestHashFileExpandsHomeDirectory(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.Mkdir(filepath.Join(cwd, "~"), 0700); err != nil {
		t.Fatal(err)
	}
	for file, text := range map[string]string{
		filepath.Join(home, "x.txt"):     "hello",
		filepath.Join(cwd, "~", "x.txt"): "shadow",
	} {
		if err := os.WriteFile(file, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(cwd)
	cases := []struct{ file, text string }{
		{"~/x.txt", "hello"},
		{"./~/x.txt", "shadow"},
		{"~" + string(os.PathSeparator) + "x.txt", "hello"},
	}
	for _, tc := range cases {
		result, err := RunLocal(Request{Tool: "hash", Options: map[string]string{"file": tc.file}})
		if err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("%x", sha256.Sum256([]byte(tc.text))); result.Output != want {
			t.Errorf("hash of %q = %q, want %q", tc.file, result.Output, want)
		}
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if _, err := RunLocal(Request{Tool: "hash", Options: map[string]string{"file": "~/x.txt"}}); err == nil || !strings.Contains(err.Error(), "expand home directory") {
		t.Errorf("hash with no home directory = %v, want an expansion error", err)
	}
}

func TestHashFilePreservesSymlinkTraversalAfterHomePrefix(t *testing.T) {
	home, target := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(target, "dir")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for file, text := range map[string]string{
		filepath.Join(home, "fixture.txt"):   "home",
		filepath.Join(target, "fixture.txt"): "target",
	} {
		if err := os.WriteFile(file, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(dir, filepath.Join(home, "link")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create a symlink: %v", err)
		}
		t.Fatal(err)
	}
	checksum := func(file string) string {
		t.Helper()
		result, err := RunLocal(Request{Tool: "hash", Options: map[string]string{"file": file}})
		if err != nil {
			t.Fatal(err)
		}
		return result.Output
	}
	const suffix = "/link/../fixture.txt"
	if got, want := checksum("~"+suffix), checksum(home+suffix); got != want {
		t.Errorf("home-relative hash = %q, equivalent absolute path = %q", got, want)
	}
}

func TestJSONToolRejectsEmptyInput(t *testing.T) {
	_, err := RunLocal(Request{Tool: "json", Input: "  "})
	if err == nil || err.Error() != "invalid JSON: input is empty" {
		t.Fatalf("empty JSON error = %v, want invalid JSON: input is empty", err)
	}
}

func TestCronRoundsIntervalsToWholeSeconds(t *testing.T) {
	result, err := RunLocal(Request{Tool: "cron", Input: "0 0 * * *", Options: map[string]string{"from": "2026-01-01T10:00:00.125Z", "zone": "UTC", "count": "2"}})
	if err != nil {
		t.Fatal(err)
	}
	if rows := result.Table.Rows; len(rows) != 2 || rows[0][2] != "14h0m0s" || rows[1][2] != "24h0m0s" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestSpeedRowsOnlyListUploadWhenMeasured(t *testing.T) {
	base := SpeedData{Servers: []string{"a", "b"}, DownloadedBytes: 1 << 20, DownloadSeconds: 2, DownloadStopReason: "duration"}
	rows := speedRows(base)
	if len(rows) != 4 {
		t.Fatalf("download-only rows = %#v", rows)
	}
	for _, r := range rows {
		if strings.HasPrefix(r.Label, "Upload") {
			t.Fatalf("unexpected upload row %#v", r)
		}
	}
	withUpload := base
	withUpload.UploadStopReason = "duration"
	if rows := speedRows(withUpload); len(rows) != 7 {
		t.Fatalf("upload rows = %#v", rows)
	}
}

func TestPageTitleCollapsesWhitespace(t *testing.T) {
	if got := pageTitle("<html><title>\n  Hello &amp;\n   World </title>"); got != "Hello & World" {
		t.Fatalf("pageTitle = %q, want Hello & World", got)
	}
	if got := pageTitle("<p>x</p>"); got != "" {
		t.Fatalf("pageTitle without title = %q, want empty", got)
	}
}
