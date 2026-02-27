// Package cmd implements the revoco CLI using cobra.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cookiespkg "github.com/fulgidus/revoco/cookies"
	"github.com/fulgidus/revoco/engine"
	"github.com/fulgidus/revoco/secrets"
	"github.com/fulgidus/revoco/session"
)

var (
	flagNoTUI      bool
	flagSource     string
	flagDest       string
	flagDryRun     bool
	flagMove       bool
	flagCookies    string
	flagInput      string
	flagOutput     string
	flagConcurrent int
	flagDelay      float64
	flagRetry      int
	flagStartFrom  int
	flagPassword   string
	flagChromeDB   string
	flagCookieOut  string
	flagSession    string // --session name
)

// rootCmd is the base command (revoco process).
var rootCmd = &cobra.Command{
	Use:   "revoco",
	Short: "Google Photos Takeout processor & recovery tool",
	Long: `revoco organizes Google Photos Takeout archives and recovers missing files.

Run without flags to launch the interactive TUI.
Use --no-tui together with --source / --dest for headless / CI use.
Use --session to work within a named session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagNoTUI {
			return runProcessHeadless()
		}
		// Launch TUI — handled in main.go via RunTUI()
		return nil
	},
}

// processCmd runs the 8-phase processing pipeline.
var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Process a Google Photos Takeout archive",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProcessHeadless()
	},
}

// recoverCmd runs the recovery download pipeline.
var recoverCmd = &cobra.Command{
	Use:   "recover",
	Short: "Download missing files listed in missing-files.json",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRecoverHeadless()
	},
}

// cookiesCmd decrypts Chrome cookies to a Netscape jar.
var cookiesCmd = &cobra.Command{
	Use:   "cookies",
	Short: "Decrypt Chrome cookies to a Netscape cookie jar",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDecryptCookies()
	},
}

// sessionCmd manages sessions from the CLI.
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage revoco sessions",
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessions, err := session.ListSessions()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions. Create one with: revoco session create <name>")
			return nil
		}
		for _, s := range sessions {
			source := s.Config.Source.OriginalPath
			if source == "" {
				source = "(no source)"
			}
			fmt.Printf("  %-30s [%s]  %s\n", s.Config.Name, s.Config.Status, source)
		}
		return nil
	},
}

var sessionCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := session.Create(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Created session %q at %s\n", s.Config.Name, s.Dir)
		return nil
	},
}

var sessionRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a session",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := session.Rename(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Renamed %q -> %q\n", args[0], args[1])
		return nil
	},
}

var sessionRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Delete a session and all its data",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := session.Remove(args[0]); err != nil {
			return err
		}
		fmt.Printf("Removed session %q\n", args[0])
		return nil
	},
}

func init() {
	// process flags
	processCmd.Flags().StringVar(&flagSource, "source", "", "Takeout source directory (required)")
	processCmd.Flags().StringVar(&flagDest, "dest", "./processed", "Destination directory")
	processCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would happen without making changes")
	processCmd.Flags().BoolVar(&flagMove, "move", false, "Move files instead of copying")
	processCmd.Flags().StringVar(&flagSession, "session", "", "Session name (logs + output go to session dir)")
	_ = processCmd.MarkFlagRequired("source")

	// recover flags
	recoverCmd.Flags().StringVar(&flagCookies, "cookies", "./google-cookies.txt", "Netscape cookie jar path")
	recoverCmd.Flags().StringVar(&flagInput, "input", "./processed/missing-files.json", "missing-files.json path")
	recoverCmd.Flags().StringVar(&flagOutput, "output", "./recovered", "Output directory")
	recoverCmd.Flags().IntVar(&flagConcurrent, "concurrency", 3, "Parallel downloads")
	recoverCmd.Flags().Float64Var(&flagDelay, "delay", 1.0, "Delay between requests (seconds)")
	recoverCmd.Flags().IntVar(&flagRetry, "retry", 3, "Max retries per file")
	recoverCmd.Flags().IntVar(&flagStartFrom, "start-from", 1, "Resume from entry N (1-indexed)")
	recoverCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would be downloaded")
	recoverCmd.Flags().StringVar(&flagSession, "session", "", "Session name (logs go to session dir)")

	// cookies flags
	cookiesCmd.Flags().StringVar(&flagPassword, "password", "", "Chrome Safe Storage password (v11 key material)")
	cookiesCmd.Flags().StringVar(&flagChromeDB, "db", "", "Path to Chrome cookies SQLite file (auto-detected if empty)")
	cookiesCmd.Flags().StringVar(&flagCookieOut, "output", "./google-cookies.txt", "Output Netscape jar path")

	// root --no-tui flag
	rootCmd.PersistentFlags().BoolVar(&flagNoTUI, "no-tui", false, "Run headless (no TUI)")

	// session subcommands
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionCreateCmd)
	sessionCmd.AddCommand(sessionRenameCmd)
	sessionCmd.AddCommand(sessionRemoveCmd)

	rootCmd.AddCommand(processCmd)
	rootCmd.AddCommand(recoverCmd)
	rootCmd.AddCommand(cookiesCmd)
	rootCmd.AddCommand(sessionCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// NeedsTUI returns true when no subcommand was given and --no-tui is not set.
func NeedsTUI() bool {
	return !flagNoTUI && len(os.Args) == 1
}

// resolveSessionDir loads the session and returns its Dir if --session is set.
func resolveSessionDir() string {
	if flagSession == "" {
		return ""
	}
	s, err := session.Load(flagSession)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load session %q: %v\n", flagSession, err)
		return ""
	}
	return s.Dir
}

func runProcessHeadless() error {
	if flagSource == "" {
		return fmt.Errorf("--source is required")
	}
	cfg := engine.PipelineConfig{
		SourceDir:  flagSource,
		DestDir:    flagDest,
		SessionDir: resolveSessionDir(),
		UseMove:    flagMove,
		DryRun:     flagDryRun,
	}
	events := make(chan engine.ProgressEvent, 64)
	go func() {
		for ev := range events {
			if ev.Total > 0 {
				fmt.Printf("[Phase %d] %s: %d/%d", ev.Phase, ev.Label, ev.Done, ev.Total)
			} else {
				fmt.Printf("[Phase %d] %s", ev.Phase, ev.Label)
			}
			if ev.Message != "" {
				fmt.Printf("  — %s", ev.Message)
			}
			fmt.Println()
		}
	}()

	result, err := engine.Run(cfg, events)
	if err != nil {
		return err
	}
	s := result.Stats
	fmt.Printf("\nDone. media=%d albums=%d dedup=%d transferred=%d exif=%d errors=%d\n",
		s.MediaFound, s.Albums, s.DuplicatesRemoved, s.FilesTransferred, s.EXIFApplied, s.Errors)
	if result.Report != nil && len(result.Report.Entries) > 0 {
		fmt.Printf("Missing files: %d — see %s\n", len(result.Report.Entries), result.Report.Path)
	}
	return nil
}

func runRecoverHeadless() error {
	cfg := engine.RecoverConfig{
		InputJSON:   flagInput,
		OutputDir:   flagOutput,
		SessionDir:  resolveSessionDir(),
		CookieJar:   flagCookies,
		Concurrency: flagConcurrent,
		Delay:       flagDelay,
		MaxRetry:    flagRetry,
		StartFrom:   flagStartFrom,
		DryRun:      flagDryRun,
	}
	events := make(chan engine.RecoverEvent, 64)
	go func() {
		for ev := range events {
			fmt.Printf("\r[%d/%d] %s   ", ev.Done, ev.Total, ev.Message)
		}
		fmt.Println()
	}()

	result, err := engine.RunRecover(cfg, events)
	if err != nil {
		return err
	}
	fmt.Printf("\nRecovery done. downloaded=%d skipped=%d failed=%d\n",
		result.Downloaded, result.Skipped, result.Failed)
	if result.FailedPath != "" {
		fmt.Printf("Failed list: %s\n", result.FailedPath)
	}
	return nil
}

func runDecryptCookies() error {
	dbPath := flagChromeDB
	if dbPath == "" {
		var err error
		dbPath, err = cookiespkg.DefaultChromeDBPath()
		if err != nil {
			return err
		}
	}

	password := flagPassword

	// If no password flag, try to read from vault
	if password == "" {
		vaultPath, err := secrets.DefaultPath()
		if err == nil && secrets.Exists(vaultPath) {
			vaultPw, err := secrets.PromptPassword("Vault password: ")
			if err == nil {
				stored, err := secrets.Get(vaultPath, vaultPw, "chrome_v11_password")
				if err == nil {
					password = stored
					fmt.Println("Using Chrome password from vault")
				}
			}
		}
	}

	count, err := cookiespkg.ExtractToJar(dbPath, password, flagCookieOut)
	if err != nil {
		return err
	}
	fmt.Printf("Decrypted %d cookies -> %s\n", count, flagCookieOut)
	return nil
}
