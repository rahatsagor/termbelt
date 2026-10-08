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
