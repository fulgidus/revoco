# revoco

**revoco** is a terminal UI (TUI) tool for processing [Google Photos Takeout](https://takeout.google.com) archives and recovering missing files.

![revoco logo](logo/revoco.png)

---

## Features

- **Process Takeout** — scan a Takeout archive, fix metadata (EXIF dates, GPS), handle motion photos, and copy everything into a clean organised destination tree
- **Recover Missing** — download files that Google omitted from the export, using cookies exported from Chrome
- **Pre-flight analysis** — before committing to a long run, get a summary: file count, match rate, albums detected, date range, estimated output size, and motion-photo count
- **Full-screen TUI** — animated spinners, smooth progress bars, an MB/s sparkline throughput graph, a live log panel, and a filterable missing-files report table
- **Mouse support** — click menu items and buttons directly
- **Headless / CI mode** — `--no-tui` flag or subcommands for scripted use
- **Self-contained** — `exiftool` and `ffmpeg` can be bundled inside the binary at build time (no runtime dependencies)

---

## Installation

### Pre-built binaries

Download the latest release from the [Releases](https://github.com/fulgidus/revoco/releases) page.

### Build from source

Requires Go 1.21+.

```sh
# Dev build (uses exiftool / ffmpeg from PATH)
make build

# Production build — fetches and embeds exiftool + ffmpeg
make bundle        # current platform only
make bundle-all    # all platforms (Linux amd64 only for now)
make release       # cross-compile linux/darwin/windows amd64 + darwin arm64
```

---

## Usage

```sh
# Launch the interactive TUI (default)
./revoco

# Process a Takeout archive non-interactively
./revoco process --source ~/Takeout --dest ~/Photos

# Recover missing files
./revoco recover --cookies cookies.txt --input missing-files.json

# Run any command without the TUI
./revoco --no-tui process --source ~/Takeout --dest ~/Photos
```

### Keyboard shortcuts (TUI)

| Key | Action |
|-----|--------|
| `↑` / `↓` or `j` / `k` | Navigate menu / fields |
| `Enter` | Select / confirm |
| `Tab` | Next field |
| `Ctrl+O` | Open folder/file browser |
| `Esc` / `q` | Back / quit |
| `Ctrl+C` | Force quit |

---

## How it works

1. **Process Takeout** reads each album folder, matches media files to their `.json` metadata sidecar, applies EXIF date/GPS fixes via `exiftool`, extracts the video track from motion photos via `ffmpeg`, and writes the result to the destination directory.
2. **Recover Missing** reads a `missing-files.json` list produced by the process step, then downloads each file from Google Photos using the provided Netscape-format cookie file.

Cookie decryption is handled natively: DPAPI + App-Bound v20 on Windows, Keychain on macOS, KWallet on Linux.

---

## Project structure

```
cmd/          Cobra CLI entry points
cookies/      Chrome cookie extraction & decryption (all platforms)
engine/       Core processing pipeline, EXIF, motion photos, recovery
internal/     Bundled-binary embed + extraction helpers
tui/          Bubble Tea screens and reusable components
legacy/       Original Bash / Node.js scripts (kept for reference)
Makefile      Build, bundle, and release targets
```

---

## License

MIT
