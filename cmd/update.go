// Package cmd implements the revoco CLI using cobra.
package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fulgidus/revoco/config"
	"github.com/fulgidus/revoco/internal/update"
	"github.com/fulgidus/revoco/internal/version"
	"github.com/spf13/cobra"
)

// GitHub release API types
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []githubAsset `json:"assets"`
	Body        string        `json:"body"`
	PublishedAt time.Time     `json:"published_at"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

const (
	githubOwner = "fulgidus"
	githubRepo  = "revoco"
	githubAPI   = "https://api.github.com"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install updates",
	Long: `Check for revoco updates and optionally install them.

Without flags, checks for available updates.
With --install, downloads and installs the latest version.

Examples:
  revoco update           # Check for updates
  revoco update --install # Install the latest version
`,
	RunE: runUpdate,
}

var (
	flagUpdateInstall bool
	flagUpdateForce   bool
	flagUpdateChannel string
)

func init() {
	updateCmd.Flags().BoolVar(&flagUpdateInstall, "install", false, "Install the update")
	updateCmd.Flags().BoolVar(&flagUpdateForce, "force", false, "Force update even if on latest version")
	updateCmd.Flags().StringVar(&flagUpdateChannel, "channel", "", "Update channel (stable/dev), overrides config")

	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	currentVersion := GetVersion()

	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Println("Checking for updates...")

	// Determine channel (flag overrides config)
	channel := flagUpdateChannel
	if channel == "" {
		cfg, err := config.Load()
		if err == nil {
			channel = cfg.Updates.Channel
		} else {
			channel = "stable" // Default fallback
		}
	}

	// Validate channel
	if err := config.ValidateChannel(channel); err != nil {
		return fmt.Errorf("invalid channel: %w", err)
	}

	// Fetch latest release using channel-aware dispatcher
	release, err := fetchLatestRelease(ctx, channel)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentClean := strings.TrimPrefix(currentVersion, "v")

	fmt.Printf("Latest version:  %s\n", latestVersion)

	// Use semver comparison instead of string equality
	isNewer, err := version.IsNewer(latestVersion, currentClean)
	if err != nil {
		return fmt.Errorf("version comparison failed: %w", err)
	}

	// Check for dev-to-stable downgrade scenario
	if version.IsDevVersion(currentVersion) && !version.IsDevVersion(latestVersion) {
		if !isNewer {
			fmt.Println("\n⚠️  Warning: You are on a newer dev version. Installing this stable version will be a downgrade.")
			fmt.Printf("    Current: %s (dev)\n", currentVersion)
			fmt.Printf("    Latest stable: %s\n", latestVersion)
		}
	}

	if !isNewer && !flagUpdateForce {
		fmt.Println("\nYou are already on the latest version.")
		return nil
	}

	if currentClean == "dev" {
		fmt.Println("\nYou are running a development version.")
		fmt.Println("Use --force to update to the latest release.")
		if !flagUpdateForce && !flagUpdateInstall {
			return nil
		}
	}

	// Show release notes
	if release.Body != "" {
		fmt.Println("\nRelease notes:")
		// Limit to first 500 chars
		body := release.Body
		if len(body) > 500 {
			body = body[:497] + "..."
		}
		fmt.Println(body)
	}

	if !flagUpdateInstall {
		fmt.Println("\nRun 'revoco update --install' to install the update.")
		return nil
	}

	// Find the appropriate asset for this platform
	asset, checksum, err := findAssetForPlatform(release)
	if err != nil {
		return err
	}

	fmt.Printf("\nDownloading %s...\n", asset.Name)

	// Download to temp file
	tmpFile, err := downloadAsset(ctx, asset)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer os.Remove(tmpFile)

	// Verify checksum if available
	if checksum != "" {
		fmt.Println("Verifying checksum...")
		if err := verifyChecksum(tmpFile, checksum); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
		fmt.Println("Checksum verified.")
	}

	// Install
	fmt.Println("Installing...")
	if err := installBinary(tmpFile); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Println("\nUpdate complete! Please restart revoco.")
	return nil
}

// fetchLatestRelease fetches the latest release from GitHub using the specified channel.
// It converts the internal/update.Release type to githubRelease for compatibility with
	// existing download/install pipeline.
func fetchLatestRelease(ctx context.Context, channel string) (*githubRelease, error) {
	// Use channel-aware dispatcher from internal/update
	release, err := update.FetchLatestRelease(ctx, githubAPI, githubOwner, githubRepo, channel)
	if err != nil {
		return nil, err
	}

	// Convert update.Release to githubRelease
	// The structs are identical, just map field by field
	assets := make([]githubAsset, len(release.Assets))
	for i, a := range release.Assets {
		assets[i] = githubAsset{
			Name:               a.Name,
			BrowserDownloadURL: a.BrowserDownloadURL,
			Size:               a.Size,
		}
	}

	return &githubRelease{
		TagName:     release.TagName,
		Name:        release.Name,
		Draft:       release.Draft,
		Prerelease:  release.Prerelease,
		Assets:      assets,
		Body:        release.Body,
		PublishedAt: release.PublishedAt,
	}, nil
}

// findAssetForPlatform finds the appropriate asset for the current platform.
func findAssetForPlatform(release *githubRelease) (*githubAsset, string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map architecture names
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
		"386":   "386",
	}

	arch, ok := archMap[goarch]
	if !ok {
		return nil, "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	// Build expected asset name pattern
	// Format: revoco_<os>_<arch>.tar.gz or revoco_<os>_<arch>.zip
	var expectedName string
	checksumName := "checksums.txt"

	switch goos {
	case "windows":
		expectedName = fmt.Sprintf("revoco_%s_%s_%s.zip", strings.TrimPrefix(release.TagName, "v"), goos, arch)
	case "darwin", "linux":
		expectedName = fmt.Sprintf("revoco_%s_%s_%s.tar.gz", strings.TrimPrefix(release.TagName, "v"), goos, arch)
	default:
		return nil, "", fmt.Errorf("unsupported OS: %s", goos)
	}

	var foundAsset *githubAsset
	var checksumAsset *githubAsset

	for _, asset := range release.Assets {
		if asset.Name == expectedName {
			a := asset
			foundAsset = &a
		}
		if asset.Name == checksumName {
			a := asset
			checksumAsset = &a
		}
	}

	if foundAsset == nil {
		return nil, "", fmt.Errorf("no asset found for %s/%s (looking for %s)", goos, goarch, expectedName)
	}

	// Get checksum if available
	checksum := ""
	if checksumAsset != nil {
		checksum, _ = getChecksumForFile(checksumAsset, expectedName)
	}

	return foundAsset, checksum, nil
}

// getChecksumForFile extracts the checksum for a specific file from the checksums file.
func getChecksumForFile(checksumAsset *githubAsset, filename string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", checksumAsset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse checksums file (format: "checksum  filename")
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == filename {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("checksum not found for %s", filename)
}

// downloadAsset downloads an asset to a temporary file.
func downloadAsset(ctx context.Context, asset *githubAsset) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "revoco-update-*")
	if err != nil {
		return "", err
	}

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()

	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// verifyChecksum verifies the SHA256 checksum of a file.
func verifyChecksum(path string, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

// installBinary installs the downloaded binary.
func installBinary(archivePath string) error {
	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return err
	}

	// Extract archive to temp directory
	tmpDir, err := os.MkdirTemp("", "revoco-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractArchiveForUpdate(archivePath, tmpDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Find the binary in the extracted files
	binaryName := "revoco"
	if runtime.GOOS == "windows" {
		binaryName = "revoco.exe"
	}

	var newBinaryPath string
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == binaryName && !info.IsDir() {
			newBinaryPath = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return err
	}

	if newBinaryPath == "" {
		return fmt.Errorf("binary not found in archive")
	}

	// Platform-specific installation
	if runtime.GOOS == "windows" {
		return installOnWindows(newBinaryPath, execPath)
	}

	return installOnUnix(newBinaryPath, execPath)
}

// installOnUnix installs on Unix-like systems (Linux, macOS).
func installOnUnix(newPath, destPath string) error {
	// Create backup
	backupPath := destPath + ".bak"
	if err := os.Rename(destPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Copy new binary
	if err := copyFileForUpdate(newPath, destPath); err != nil {
		// Restore backup
		os.Rename(backupPath, destPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(destPath, 0755); err != nil {
		os.Rename(backupPath, destPath)
		return err
	}

	// Remove backup
	os.Remove(backupPath)

	return nil
}

// installOnWindows installs on Windows using rename trick.
func installOnWindows(newPath, destPath string) error {
	// On Windows, we can't replace a running executable directly.
	// Instead, we rename it and copy the new one.
	backupPath := destPath + ".old"

	// Remove old backup if exists
	os.Remove(backupPath)

	// Rename current to .old
	if err := os.Rename(destPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Copy new binary
	if err := copyFileForUpdate(newPath, destPath); err != nil {
		// Restore backup
		os.Rename(backupPath, destPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Note: We don't remove the .old file as the current process is still using it.
	// It will be cleaned up on next update or can be manually removed after restart.
	fmt.Println("Note: Old version backed up to", backupPath)
	fmt.Println("You can delete it after verifying the new version works.")

	return nil
}

// extractArchiveForUpdate extracts an archive (tar.gz or zip).
func extractArchiveForUpdate(archivePath, destDir string) error {
	// Determine archive type by examining content
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}

	// Read magic bytes
	magic := make([]byte, 4)
	_, err = f.Read(magic)
	f.Close()
	if err != nil {
		return err
	}

	// Check for gzip magic (1f 8b)
	if magic[0] == 0x1f && magic[1] == 0x8b {
		return extractTarGzForUpdate(archivePath, destDir)
	}

	// Check for ZIP magic (50 4b 03 04)
	if magic[0] == 0x50 && magic[1] == 0x4b && magic[2] == 0x03 && magic[3] == 0x04 {
		return extractZipForUpdate(archivePath, destDir)
	}

	return fmt.Errorf("unsupported archive format")
}

// extractTarGzForUpdate extracts a tar.gz archive.
func extractTarGzForUpdate(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)

		// Check for path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}

			outFile, err := os.Create(target)
			if err != nil {
				return err
			}

			_, err = io.Copy(outFile, tarReader)
			outFile.Close()

			if err != nil {
				return err
			}

			// Preserve executable bit
			if header.Mode&0111 != 0 {
				os.Chmod(target, 0755)
			}
		}
	}

	return nil
}

// extractZipForUpdate extracts a ZIP archive.
func extractZipForUpdate(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)

		// Check for path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}

		// Preserve executable bit
		if f.Mode()&0111 != 0 {
			os.Chmod(target, 0755)
		}
	}

	return nil
}

// copyFileForUpdate copies a file from src to dst.
func copyFileForUpdate(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
