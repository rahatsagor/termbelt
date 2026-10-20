package cli

import (
	"strings"
	"testing"
)

func TestTimeReadsPipedInput(t *testing.T) {
	out, err := execute(t, []string{"time", "--zone", "UTC", "--json"}, "1700000000\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2023-11-14") {
		t.Fatalf("time ignored piped input: %s", out)
	}
}
