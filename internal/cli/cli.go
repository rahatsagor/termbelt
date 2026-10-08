package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/rahatsagor/termbelt/internal/core"
	"github.com/rahatsagor/termbelt/internal/render"
	"github.com/rahatsagor/termbelt/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

func Execute(version string) error { root := NewCommand(version); return root.Execute() }
func NewCommand(version string) *cobra.Command {
	var jsonOutput, plain, raw bool
	var timeout int
	var ipAPI string
	root := &cobra.Command{Use: "termbelt", Short: "Your developer toolkit. One terminal, fewer tabs.", Long: "TERMBELT · your daily developer toolkit\n\nLaunch the interactive workbench with termbelt, or call any utility directly.\nAll utilities support --json for scripting. Local tools accept piped input.", Version: version, SilenceUsage: true, SilenceErrors: true}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Emit structured JSON")
	root.PersistentFlags().BoolVar(&plain, "plain", false, "Disable colors")
	root.PersistentFlags().BoolVar(&raw, "raw", false, "Print only output / data (for piping)")
	root.PersistentFlags().IntVar(&timeout, "timeout", 0, "Network request timeout in seconds (3–120)")
	root.PersistentFlags().StringVar(&ipAPI, "ip-api-url", "", "Use a compatible custom IP lookup API")
	root.MarkFlagsMutuallyExclusive("json", "raw")
	load := func() (*core.Engine, error) {
		config, err := core.LoadConfig()
		if err != nil {
			return nil, err
		}
		if timeout != 0 {
			if timeout < 3 || timeout > 120 {
				return nil, fmt.Errorf("timeout must be 3–120 seconds")
			}
			config.TimeoutSeconds = timeout
		}
		if ipAPI != "" {
			if _, err := core.ValidateIPAPI(ipAPI); err != nil {
				return nil, err
			}
			config.IPAPIURL = strings.TrimRight(ipAPI, "/")
		}
		return core.NewEngine(config), nil
	}
	launch := func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown command %q; run termbelt --help", args[0])
		}
		if jsonOutput || raw {
			return fmt.Errorf("--json and --raw require a utility command, such as termbelt ip --json")
		}
		if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
			return cmd.Help()
		}
		engine, err := load()
		if err != nil {
			return err
		}
		return tui.Run(engine, version, plain)
	}
	root.RunE = launch
	root.AddCommand(&cobra.Command{Use: "tui", Short: "Open the interactive workbench", Args: cobra.NoArgs, RunE: launch})
	for _, tool := range core.Tools {
		tool := tool
		cmd := &cobra.Command{Use: tool.ID + " [input]", Short: tool.Description, Long: tool.Name + "\n\n" + tool.Hint, Example: tool.Example, DisableFlagsInUseLine: true, Args: func(cmd *cobra.Command, args []string) error {
			switch tool.ID {
			case "speed", "ports", "uuid", "password":
				return cobra.NoArgs(cmd, args)
			}
			if tool.ID == "regex" {
				if len(args) > 2 {
					return fmt.Errorf("usage: termbelt regex PATTERN [TEXT]")
				}
				return nil
			}
			if len(args) > 1 {
				return fmt.Errorf("pass the input as one quoted argument, or pipe it on stdin")
			}
			return nil
		}}
		for _, field := range tool.Fields {
			if field.Key == "input" || field.Key == "pattern" {
				continue
			}
			if field.Key == "count" || field.Key == "length" {
				n, _ := strconv.Atoi(field.Default)
				cmd.Flags().Int(field.Key, n, field.Help)
			} else {
				cmd.Flags().String(field.Key, field.Default, nonemptyHelp(field.Help, field.Label))
			}
		}
		switch tool.ID {
		case "speed":
			cmd.Flags().Int("max-mb", 128, "Maximum MiB transferred per direction (1–1024)")
			cmd.Flags().Bool("download-only", false, "Skip Cloudflare upload measurement")
		case "domains":
			cmd.Flags().Bool("all", false, "Check every TLD in IANA's root-zone list")
			cmd.Flags().Bool("only-unregistered", false, "Show only unregistered names in the table")
			cmd.Flags().Bool("refresh", false, "Refresh IANA metadata before the check")
			cmd.Flags().Int("concurrency", 6, "Parallel lookups, with per-provider pacing (1–12)")
			cmd.MarkFlagsMutuallyExclusive("all", "tlds")
		case "dns":
			cmd.Flags().String("resolver", "", "Custom DNS resolver IP or IP:port")
		case "json":
			cmd.Flags().Bool("minify", false, "Compact the JSON output")
		case "base64":
			cmd.Flags().Bool("decode", false, "Decode Base64 input")
			cmd.Flags().Bool("url-safe", false, "Encode unpadded URL-safe Base64")
		case "url":
			cmd.Flags().Bool("decode", false, "Decode percent-encoded text")
		case "hash":
			cmd.Flags().String("file", "", "Stream a file instead of text")
		case "password":
			cmd.Flags().Bool("no-symbols", false, "Use letters and digits only")
		case "cron":
			cmd.Flags().String("from", "", "Preview from a specific date or timestamp")
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			engine, err := load()
			if err != nil {
				return err
			}
			req := core.Request{Tool: tool.ID, Options: map[string]string{}}
			cmd.Flags().VisitAll(func(f *pflag.Flag) { req.Options[f.Name] = f.Value.String() })
			if tool.ID == "regex" {
				if len(args) == 0 {
					return fmt.Errorf("a regex pattern is required")
				}
				req.Options["pattern"] = args[0]
				if len(args) == 2 {
					req.Input = args[1]
				}
			} else if len(args) > 0 {
				req.Input = args[0]
			}
			if tool.ID == "hash" && len(args) > 0 && req.Options["file"] != "" {
				return fmt.Errorf("choose a text argument or --file as the hash source")
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			needsStdin := tool.Local && tool.ID != "ports" && tool.ID != "uuid" && tool.ID != "password" && tool.ID != "time" && tool.ID != "cron" && tool.ID != "cidr"
			hasArg := len(args) > 0
			if tool.ID == "regex" {
				hasArg = len(args) > 1
			}
			if needsStdin && !hasArg && req.Options["file"] == "" && !term.IsTerminal(int(os.Stdin.Fd())) {
				input, closeInput := interruptibleInput(ctx, cmd.InOrStdin())
				defer closeInput()
				if tool.ID == "hash" {
					req.InputReader = input
				} else {
					b, err := readInput(input)
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if err != nil {
						return err
					}
					req.Input = string(b)
				}
			}
			showProgress := !jsonOutput && !raw && term.IsTerminal(int(os.Stderr.Fd()))
			var mu sync.Mutex
			var lastProgress time.Time
			progressWidth := 96
			if w, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil {
				progressWidth = max(w-4, 10)
			}
			emit := func(p core.Progress) {
				if showProgress {
					mu.Lock()
					if time.Since(lastProgress) >= 100*time.Millisecond || p.Fraction >= 1 {
						fmt.Fprintf(cmd.ErrOrStderr(), "\r\033[2K  %s", ansi.Truncate(render.Safe(p.Message), progressWidth, "…"))
						lastProgress = time.Now()
					}
					mu.Unlock()
				}
			}
			result, err := engine.Run(ctx, req, emit)
			if showProgress {
				fmt.Fprint(cmd.ErrOrStderr(), "\r\033[2K")
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if jsonOutput {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			if raw {
				if result.Output != "" {
					_, err := fmt.Fprintln(out, result.Output)
					return err
				}
				_, err := fmt.Fprintln(out, core.PrettyJSON(result.Data))
				return err
			}
			width := 100
			if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
				width = max(w-4, 30)
			}
			color := !plain && os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(os.Stdout.Fd()))
			_, err = fmt.Fprintln(out, render.Result(result, width, color))
			return err
		}
		root.AddCommand(cmd)
	}
	config := &cobra.Command{Use: "config", Short: "View settings or connect your IP API", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := core.LoadConfig()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), core.PrettyJSON(map[string]any{"path": core.ConfigPath(), "settings": c}))
		return err
	}}
	config.AddCommand(&cobra.Command{Use: "set KEY VALUE", Short: "Save ip-api-url or timeout-seconds", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := core.LoadConfig()
		if err != nil {
			return err
		}
		if err = c.Set(args[0], args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Saved to", core.ConfigPath())
		return err
	}})
	root.AddCommand(config)
	root.AddCommand(&cobra.Command{Use: "completion [bash|zsh|fish]", Short: "Generate shell completion", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
		case "zsh":
			return root.GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		default:
			return fmt.Errorf("choose bash, zsh or fish")
		}
	}})
	return root
}

// Closing os.Stdin alone does not reliably interrupt an already-blocked read
// on every platform. The bounded pipe lets cancellation wake the consumer
// immediately while retaining streaming and backpressure for large hashes.
func interruptibleInput(ctx context.Context, source io.Reader) (io.Reader, func()) {
	r, w := io.Pipe()
	go func() {
		_, err := io.Copy(w, source)
		_ = w.CloseWithError(err)
	}()
	stop := context.AfterFunc(ctx, func() {
		_ = w.CloseWithError(ctx.Err())
		if closer, ok := source.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	return r, func() { stop(); _ = r.Close() }
}

func nonemptyHelp(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func readInput(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 16<<20 {
		return nil, fmt.Errorf("stdin exceeds 16 MiB; use hash --file for large files")
	}
	return b, nil
}
