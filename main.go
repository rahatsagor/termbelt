package main

import (
	"fmt"
	"os"

	"github.com/rahatsagor/termbelt/internal/cli"
	"github.com/rahatsagor/termbelt/internal/render"
)

var version = "1.1.0"

func main() {
	if err := cli.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, "termbelt:", render.Safe(err.Error()))
		os.Exit(1)
	}
}
