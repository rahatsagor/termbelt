package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type repeatedByteReader struct {
	remaining int64
	value     byte
}

func (r *repeatedByteReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	for i := int64(0); i < n; i++ {
		p[i] = r.value
	}
	r.remaining -= n
	return int(n), nil
}

type cancelAfterRead struct {
	cancel context.CancelFunc
	done   bool
}

func (r *cancelAfterRead) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	copy(p, "payload")
	r.cancel()
	return len("payload"), nil
}

func TestAuditHashStreamsLargeInputAndHonorsCancellation(t *testing.T) {
	const size = 16<<20 + 1
	hashResult, err := hashTool(Request{Tool: "hash", Options: map[string]string{"algorithm": "sha256"}, InputReader: &repeatedByteReader{remaining: size, value: 'x'}})
	if err != nil {
		t.Fatal(err)
	}
	data := hashResult.Data.(map[string]any)
	if data["bytes"] != int64(size) {
		t.Fatalf("hashed bytes = %#v, want %d", data["bytes"], size)
	}
	want := sha256.Sum256([]byte(strings.Repeat("x", 16<<20+1)))
	if hashResult.Output != hex.EncodeToString(want[:]) {
		t.Fatalf("hash = %q, want %x", hashResult.Output, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, err = hashToolContext(ctx, Request{Tool: "hash", Options: map[string]string{"algorithm": "sha256"}, InputReader: &cancelAfterRead{cancel: cancel}})
	if err == nil || err != context.Canceled {
		t.Fatalf("cancelled hash error = %v, want context.Canceled", err)
	}
}

func TestAuditHashRejectsNonRegularFile(t *testing.T) {
	path := t.TempDir()
	_, err := RunLocal(localRequest("hash", "", map[string]string{"file": path}))
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("hash directory error = %v, want regular-file rejection", err)
	}
}

func TestAuditConfigRejectsNullOversizedAndBadPorts(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "null", body: "null"},
		{name: "oversized", body: `{"timeout_seconds":20,"padding":"` + strings.Repeat("x", 1<<20) + `"}`},
		{name: "bad port", body: `{"timeout_seconds":20,"ip_api_url":"http://127.0.0.1:99999"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("TERMBELT_CONFIG", path)
			if _, err := LoadConfig(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestAuditConfigCleansFavoritesAndMutationsPreserveDiskOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	initial := `{"ip_api_url":"https://disk.example/api","timeout_seconds":31,"favorites":["json","json","unknown"]}`
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TERMBELT_CONFIG", path)
	t.Setenv("TERMBELT_IP_API_URL", "https://environment.example/api")

	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(c.Favorites); got != "[json]" {
		t.Fatalf("favorites = %s, want deduplicated [json]", got)
	}
	if c.IPAPIURL != "https://environment.example/api" {
		t.Fatalf("LoadConfig URL override = %q", c.IPAPIURL)
	}
	if err := SaveFavorites([]string{"uuid", "uuid", "unknown"}); err != nil {
		t.Fatal(err)
	}
	stored, err := loadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if stored.IPAPIURL != "https://disk.example/api" || stored.TimeoutSeconds != 31 || fmt.Sprint(stored.Favorites) != "[uuid]" {
		t.Fatalf("SaveFavorites changed disk settings unexpectedly: %#v", stored)
	}

	if err := c.Set("timeout-seconds", "45"); err != nil {
		t.Fatal(err)
	}
	stored, err = loadConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	if stored.IPAPIURL != "https://disk.example/api" || stored.TimeoutSeconds != 45 || fmt.Sprint(stored.Favorites) != "[uuid]" {
		t.Fatalf("Config.Set changed disk URL/favorites: %#v", stored)
	}
}

func TestAuditTimestampUnitsAndNegativeFractionKeepNanoseconds(t *testing.T) {
	for _, tc := range []struct {
		input string
		unit  string
		want  time.Time
	}{
		{input: "1.000000001", unit: "s", want: time.Unix(1, 1)},
		{input: "1000.000001", unit: "ms", want: time.Unix(1, 1)},
		{input: "1000000.001", unit: "us", want: time.Unix(1, 1)},
		{input: "1000000001", unit: "ns", want: time.Unix(1, 1)},
		{input: "-0.000000001", unit: "s", want: time.Unix(-1, 999_999_999)},
	} {
		t.Run(tc.unit+"/"+tc.input, func(t *testing.T) {
			got, err := parseTimeUnit(tc.input, time.UTC, tc.unit)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(tc.want) || got.Nanosecond() != tc.want.Nanosecond() {
				t.Fatalf("parseTimeUnit = %s (%d ns), want %s (%d ns)", got, got.Nanosecond(), tc.want, tc.want.Nanosecond())
			}
		})
	}
}
