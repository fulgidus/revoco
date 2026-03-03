# revoco

![revoco logo](logo/revoco.png)

**Data liberation tool for escaping big tech walled gardens.**

> **Early Development** — This software is not yet ready for general use. Running it means contributing to its development. Expect rough edges, breaking changes, and incomplete features. Feedback and contributions welcome!

---

## What it does

Revoco helps you take back control of your data by importing from cloud services and exporting to storage you own. Currently focused on Google services (Drive, Photos Takeout), with more connectors planned.

---

## Current Features

- **Google Drive connector** — OAuth2 authentication, file listing, Google Docs/Sheets/Slides export to local formats
- **Google Photos Takeout processing** — EXIF metadata repair, GPS coordinates, motion photo extraction
- **Local connectors** — Folder, ZIP, and TGZ archive import with copy/move/reference modes
- **Connection testing** — Verify connector access before starting operations
- **Full-screen TUI** — Dashboard, connector wizards, progress tracking, cancellation support
- **Self-contained builds** — Optional `exiftool` and `ffmpeg` bundling

---

## Architecture

### Connectors

Modular data sources and destinations. Each connector can act as:
- **Input** — Primary data source for import
- **Output** — Destination for processed data  
- **Fallback** — Secondary source for repairing missing files

### Processors

Transform data between import and export: metadata extraction, format conversion, deduplication, organization.

### Sessions

Track import/export operations with versioned schemas, connector configurations, and processing state.

---

## Installation

### Quick Install (Linux/macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/fulgidus/revoco/main/install.sh | bash
```

### macOS (Homebrew)

```sh
brew tap fulgidus/revoco && brew install revoco
```

### Linux

#### Homebrew

```sh
brew tap fulgidus/revoco && brew install revoco
```

#### Debian / Ubuntu (apt)

Download the `.deb` package from [GitHub Releases](https://github.com/fulgidus/revoco/releases).

```sh
sudo dpkg -i revoco_*.deb
```

#### Fedora / RHEL (dnf)

Download the `.rpm` package from [GitHub Releases](https://github.com/fulgidus/revoco/releases).

```sh
sudo rpm -i revoco_*.rpm
```

#### Arch Linux (AUR)

```sh
yay -S revoco-bin  # or paru -S revoco-bin
```

### Windows

#### Scoop

```sh
scoop bucket add revoco https://github.com/fulgidus/scoop-revoco && scoop install revoco
```

#### Chocolatey

```sh
choco install revoco
```

#### Winget

```sh
winget install fulgidus.revoco
```

### Container (Docker)

```sh
docker pull ghcr.io/fulgidus/revoco:latest
docker run -it --rm -v "$(pwd):/data" ghcr.io/fulgidus/revoco:latest
```

### From Source

#### Go Install

```sh
go install github.com/fulgidus/revoco@latest
```

#### Build Manual

```sh
# Requires Go 1.23+
git clone https://github.com/fulgidus/revoco.git
cd revoco
make build

# Or with bundled exiftool + ffmpeg
make bundle
```

---

## Usage

```sh
# Launch TUI
revoco

# Non-interactive processing
revoco process --source ~/Takeout --dest ~/Photos

# Check for updates
revoco update

# Install the latest version
revoco update --install
```

### Plugin Management

```sh
# List installed plugins
revoco plugins list

# Search for plugins
revoco plugins search csv

# Install a plugin
revoco plugins install <plugin-id>

# Update plugins
revoco plugins update --all
```

### Configuration

```sh
# Show current config
revoco config show

# Enable automatic update checks
revoco config set updates.auto-check true

# Enable automatic plugin updates
revoco config set plugins.auto-update true
```

---

## Uninstallation

### Quick Uninstall (Interactive)

```sh
curl -fsSL https://raw.githubusercontent.com/fulgidus/revoco/main/uninstall.sh | bash
```

### Non-interactive Full Removal

```sh
curl -fsSL https://raw.githubusercontent.com/fulgidus/revoco/main/uninstall.sh | bash -s -- --yes
```

The uninstaller will prompt to remove:
- **Binary** — The revoco executable
- **Config** — `~/.config/revoco/` (settings and plugin config)
- **Plugins** — `~/.config/revoco/plugins/` (installed plugins)
- **Sessions** — `~/.revoco/sessions/` (your work data — **use caution**)
- **Cache** — `~/.cache/revoco/` (cached tools like exiftool)

---

## License

GPL-3.0
