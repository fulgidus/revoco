// Package cmd implements the revoco CLI using cobra.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cookiespkg "github.com/fulgidus/revoco/cookies"
	"github.com/fulgidus/revoco/engine"
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
)

// rootCmd is the base command (revoco process).
var rootCmd = &cobra.Command{
	Use:   "revoco",
	Short: "Google Photos Takeout processor & recovery tool",
	Long: `revoco organizes Google Photos Takeout archives and recovers missing files.

Run without flags to launch the interactive TUI.
Use --no-tui together with --source / --dest for headless / CI use.`,
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

func init() {
	// process flags
	processCmd.Flags().StringVar(&flagSource, "source", "", "Takeout source directory (required)")
	processCmd.Flags().StringVar(&flagDest, "dest", "./processed", "Destination directory")
	processCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would happen without making changes")
	processCmd.Flags().BoolVar(&flagMove, "move", false, "Move files instead of copying")
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

	// cookies flags
	cookiesCmd.Flags().StringVar(&flagPassword, "password", "", "Chrome Safe Storage password (v11 key material)")
	cookiesCmd.Flags().StringVar(&flagChromeDB, "db", "", "Path to Chrome cookies SQLite file (auto-detected if empty)")
	cookiesCmd.Flags().StringVar(&flagCookieOut, "output", "./google-cookies.txt", "Output Netscape jar path")

	// root --no-tui flag
	rootCmd.PersistentFlags().BoolVar(&flagNoTUI, "no-tui", false, "Run headless (no TUI)")

	rootCmd.AddCommand(processCmd)
	rootCmd.AddCommand(recoverCmd)
	rootCmd.AddCommand(cookiesCmd)
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

func runProcessHeadless() error {
	if flagSource == "" {
		return fmt.Errorf("--source is required")
	}
	cfg := engine.PipelineConfig{
		SourceDir: flagSource,
		DestDir:   flagDest,
		UseMove:   flagMove,
		DryRun:    flagDryRun,
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
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home dir: %w", err)
		}
		dbPath = home + "/.config/google-chrome/Default/Cookies"
	}

	dec := cookiespkg.NewChromeDecryptor(flagPassword)
	rows, err := cookiespkg.ReadChromeCookies(dbPath, cookiespkg.GoogleDomains, dec)
	if err != nil {
		return fmt.Errorf("read cookies: %w", err)
	}
	if err := cookiespkg.WriteNetscapeJar(flagCookieOut, rows); err != nil {
		return fmt.Errorf("write jar: %w", err)
	}
	fmt.Printf("Decrypted %d cookies → %s\n", len(rows), flagCookieOut)
	return nil
}
