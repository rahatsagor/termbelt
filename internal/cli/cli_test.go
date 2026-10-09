package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func execute(t *testing.T, args []string, input string) (string, error) {
	t.Helper()
	t.Setenv("TERMBELT_CONFIG", t.TempDir()+"/config.json")
	cmd := NewCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}
func TestStructuredJSONAndPipedInput(t *testing.T) {
	out, err := execute(t, []string{"json", "--json"}, `{"big":9007199254740993}`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err = json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %q", out)
	}
	if result["tool"] != "json" || !strings.Contains(result["output"].(string), "9007199254740993") {
		t.Fatalf("output = %s", out)
	}
}
func TestRawHashForPipelines(t *testing.T) {
	out, err := execute(t, []string{"hash", "abc", "--raw"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("raw output = %q", out)
	}
}
func TestUnknownCommandAndInvalidOptionsFail(t *testing.T) {
	for _, args := range [][]string{{"does-not-exist"}, {"password", "--length", "3"}, {"regex"}, {"json", "{}", "{}"}, {"ip", "--timeout", "1"}} {
		if _, err := execute(t, args, ""); err == nil {
			t.Errorf("accepted args %v", args)
		}
	}
}
func TestGenerateZshCompletion(t *testing.T) {
	out, err := execute(t, []string{"completion", "zsh"}, "")
	if err != nil || !strings.Contains(out, "#compdef termbelt") {
		t.Fatalf("completion: %v, %s", err, out)
	}
}

func TestRawJSONStringAndOrderedOutput(t *testing.T) {
	out, err := execute(t, []string{"json", "--path", `user["display.name"]`, "--raw"}, `{"user":{"display.name":"Ada <admin>"}}`)
	if err != nil || out != "Ada <admin>\n" {
		t.Fatalf("raw string = %q, %v", out, err)
	}
	out, err = execute(t, []string{"json", "--json"}, `{"z":1,"a":"<b>"}`)
	if err != nil || !strings.Contains(out, `"data": {`) || strings.Index(out, `"z": 1`) > strings.Index(out, `"a": "<b>"`) || strings.Contains(out, `\u003c`) {
		t.Fatalf("json envelope = %s, %v", out, err)
	}
}

func TestLineInputDropsTrailingNewline(t *testing.T) {
	out, err := execute(t, []string{"url", "--raw"}, "a b\n")
	if err != nil || out != "a%20b\n" {
		t.Fatalf("url = %q, %v", out, err)
	}
	out, err = execute(t, []string{"cidr", "--json"}, "10.0.0.0/30\n")
	if err != nil || !strings.Contains(out, `"usable_hosts": "2"`) {
		t.Fatalf("cidr = %q, %v", out, err)
	}
}

func TestToolOptionsAreTypedFlags(t *testing.T) {
	out, err := execute(t, []string{"password", "--length", "16", "--count", "2", "--no-symbols", "--raw"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if len(line) != 16 || strings.ContainsAny(line, "!@#$%^&*-_+=?") {
			t.Fatalf("password %q", line)
		}
	}
	if _, err := execute(t, []string{"speed", "--max-mb", "nope"}, ""); err == nil {
		t.Fatal("non-numeric --max-mb was accepted")
	}
}

func TestConfigResetAndPath(t *testing.T) {
	t.Setenv("TERMBELT_IP_API_URL", "")
	out, err := execute(t, []string{"config", "reset"}, "")
	if err != nil || !strings.Contains(out, "Restored defaults") {
		t.Fatalf("reset = %q, %v", out, err)
	}
	if out, err = execute(t, []string{"config", "path"}, ""); err != nil || !strings.HasSuffix(strings.TrimSpace(out), "config.json") {
		t.Fatalf("path = %q, %v", out, err)
	}
}
