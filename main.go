package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/rahatsagor/termbelt/internal/cli"
	"github.com/rahatsagor/termbelt/internal/render"
)

var version = "1.2.1"

func main() {
	if err := cli.Execute(version); err != nil {
		// Only an interrupt or termination signal cancels a command; use the
		// shell convention for "stopped by the user" instead of a failure.
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "termbelt: canceled")
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "termbelt:", render.Safe(err.Error()))
		os.Exit(1)
	}
}
