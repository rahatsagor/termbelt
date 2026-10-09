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

func Execute(version string) error {
	core.SetVersion(version)
	root := NewCommand(version)
	return root.Execute()
}

// stdinModes lists tools that read piped input when no argument is given.
// "bytes" keeps input exact; "line" drops a single trailing newline, which
// echo and most editors append.
var stdinModes = map[string]string{
	"json": "bytes", "base64": "bytes", "jwt": "bytes", "hash": "bytes", "regex": "bytes",
	"url": "line", "cidr": "line", "cron": "line",
	"domains": "line", "whois": "line", "site": "line", "dns": "line", "tls": "line", "http": "line", "ping": "line",
}

func trimTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return s[:len(s)-2]
	}
	return strings.TrimSuffix(s, "\n")
}
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
	load := func(cmd *cobra.Command) (*core.Engine, error) {
		config, warning, err := core.LoadEffectiveConfig()
		if err != nil {
			return nil, err
		}
		if warning != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "termbelt: using default settings: %s\n  fix it with termbelt config set KEY VALUE or termbelt config reset\n", render.Safe(warning.Error()))
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
		engine, err := load(cmd)
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
			help := nonemptyHelp(field.Help, field.Label)
			switch field.Kind {
			case core.IntField:
				n, _ := strconv.Atoi(field.Default)
				cmd.Flags().Int(field.Key, n, help)
			case core.BoolField:
				cmd.Flags().Bool(field.Key, field.Default == "true", help)
			default:
				cmd.Flags().String(field.Key, field.Default, help)
			}
		}
		switch tool.ID {
		case "domains":
			cmd.Flags().Bool("all", false, "Check every TLD in IANA's root-zone list")
			cmd.MarkFlagsMutuallyExclusive("all", "tlds")
		case "base64":
			cmd.Flags().Bool("decode", false, "Decode Base64 input (same as --mode decode)")
		case "url":
			cmd.Flags().Bool("decode", false, "Decode percent-encoded text (same as --mode decode)")
		}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			engine, err := load(cmd)
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
			mode := stdinModes[tool.ID]
			hasArg := len(args) > 0
			if tool.ID == "regex" {
				hasArg = len(args) > 1
			}
			if mode != "" && !hasArg && req.Options["file"] == "" && !stdinIsTerminal(cmd) {
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
					if mode == "line" {
						req.Input = trimTrailingNewline(req.Input)
					}
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
				enc.SetEscapeHTML(false)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			if raw {
				if result.RawOutput != "" {
					_, err := fmt.Fprintln(out, result.RawOutput)
					return err
				}
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
	config := &cobra.Command{Use: "config", Short: "View or change settings", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, warning, err := core.LoadEffectiveConfig()
		if err != nil {
			return err
		}
		view := map[string]any{"path": core.ConfigPath(), "settings": c}
		if warning != nil {
			view["warning"] = warning.Error() + "; defaults are in use"
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), render.Safe(core.PrettyJSON(view)))
		return err
	}}
	config.AddCommand(&cobra.Command{Use: "set KEY VALUE", Short: "Save ip-api-url, timeout-seconds or favorites (comma-separated tool IDs)", Args: cobra.ExactArgs(2), ValidArgs: []string{"ip-api-url", "timeout-seconds", "favorites"}, RunE: func(cmd *cobra.Command, args []string) error {
		var c core.Config
		if err := c.Set(args[0], args[1]); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Saved to", core.ConfigPath())
		return err
	}})
	config.AddCommand(&cobra.Command{Use: "reset", Short: "Restore default settings", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := core.ResetConfig(); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Restored defaults in", core.ConfigPath())
		return err
	}})
	config.AddCommand(&cobra.Command{Use: "path", Short: "Print the settings file location", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), core.ConfigPath())
		return err
	}})
	root.AddCommand(config)
	root.AddCommand(&cobra.Command{Use: "completion [bash|zsh|fish|powershell]", Short: "Generate shell completion", Args: cobra.ExactArgs(1), ValidArgs: []string{"bash", "zsh", "fish", "powershell"}, RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
		case "zsh":
			return root.GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		default:
			return fmt.Errorf("choose bash, zsh, fish or powershell")
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

// Tests replace stdin with a reader; only the real os.Stdin can be a terminal.
func stdinIsTerminal(cmd *cobra.Command) bool {
	if f, ok := cmd.InOrStdin().(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
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
