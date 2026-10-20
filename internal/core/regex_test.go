package core

import (
	"strings"
	"testing"
)

func TestRegexSummaryShowsCappedCount(t *testing.T) {
	res, err := regexTool(Request{Tool: "regex", Input: strings.Repeat("a", 1001), Options: map[string]string{"pattern": "a"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary != "1,000+ matches" {
		t.Fatalf("summary = %q", res.Summary)
	}
}
