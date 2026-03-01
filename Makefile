# revoco Makefile
# Builds the binary and (optionally) fetches+bundles static exiftool & ffmpeg.
#
# Targets:
#   make build          — dev build, no bundled tools (falls back to PATH)
#   make bundle         — fetch tools for current platform, then build with embed
#   make bundle-all     — fetch tools for all platforms (cross-build prep)
#   make release        — bundle-all then cross-compile for linux/darwin/windows
#   make clean

BINARY   := revoco
MODULE   := github.com/fulgidus/revoco
BIN_DIR  := internal/bundled/bin

# ── version info ─────────────────────────────────────────────────────────────
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE     ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS  := -s -w \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildDate=$(DATE)

# ── tool versions ────────────────────────────────────────────────────────────
EXIFTOOL_VER  := 13.10
FFMPEG_VER    := 7.1.1

# ── exiftool static download URLs ────────────────────────────────────────────
EXIFTOOL_LINUX_AMD64   := https://exiftool.org/Image-ExifTool-$(EXIFTOOL_VER).tar.gz
EXIFTOOL_DARWIN_AMD64  := https://exiftool.org/ExifTool-$(EXIFTOOL_VER).pkg
# exiftool is a Perl script — we use the standalone Linux binary from pmmmwh/exiftool-binaries
EXIFTOOL_LINUX_BINARY  := https://github.com/pmmmwh/exiftool-binaries/releases/download/v$(EXIFTOOL_VER)/exiftool-linux-x64

# ── ffmpeg static download URLs ───────────────────────────────────────────────
FFMPEG_LINUX_AMD64  := https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl.tar.xz
FFMPEG_DARWIN_AMD64 := https://evermeet.cx/ffmpeg/ffmpeg-$(FFMPEG_VER).zip
FFMPEG_WIN_AMD64    := https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip

.PHONY: build bundle bundle-linux bundle-darwin bundle-windows bundle-all release clean

# ── dev build (no bundled tools) ─────────────────────────────────────────────
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

# ── bundle for current host platform ─────────────────────────────────────────
bundle:
	$(MAKE) bundle-$(shell go env GOOS)-$(shell go env GOARCH)
	go build -tags with_bundled_tools -ldflags "$(LDFLAGS)" -o $(BINARY) .

# ── Linux amd64 ──────────────────────────────────────────────────────────────
bundle-linux-amd64: $(BIN_DIR)/exiftool $(BIN_DIR)/ffmpeg

$(BIN_DIR)/exiftool:
	mkdir -p $(BIN_DIR)
	curl -fsSL "$(EXIFTOOL_LINUX_BINARY)" -o $(BIN_DIR)/exiftool
	chmod 0755 $(BIN_DIR)/exiftool

$(BIN_DIR)/ffmpeg:
	mkdir -p $(BIN_DIR)
	curl -fsSL "$(FFMPEG_LINUX_AMD64)" -o /tmp/ffmpeg-linux.tar.xz
	tar -xJf /tmp/ffmpeg-linux.tar.xz -C /tmp --wildcards '*/ffmpeg' --strip-components=2
	mv /tmp/ffmpeg $(BIN_DIR)/ffmpeg
	chmod 0755 $(BIN_DIR)/ffmpeg
	rm -f /tmp/ffmpeg-linux.tar.xz

# ── macOS amd64 (adjust GOARCH for arm64 on Apple Silicon) ───────────────────
bundle-darwin-amd64: bundle-darwin-ffmpeg
	@echo "exiftool on macOS: use PATH (brew install exiftool) — skipping embed"
	@test -f $(BIN_DIR)/exiftool || ( \
	  curl -fsSL "$(EXIFTOOL_LINUX_BINARY)" -o $(BIN_DIR)/exiftool && \
	  chmod 0755 $(BIN_DIR)/exiftool )

bundle-darwin-ffmpeg:
	mkdir -p $(BIN_DIR)
	curl -fsSL "$(FFMPEG_DARWIN_AMD64)" -o /tmp/ffmpeg-mac.zip
	unzip -jq /tmp/ffmpeg-mac.zip '*/ffmpeg' -d $(BIN_DIR)/
	chmod 0755 $(BIN_DIR)/ffmpeg
	rm -f /tmp/ffmpeg-mac.zip

# ── Windows amd64 ────────────────────────────────────────────────────────────
bundle-windows-amd64:
	mkdir -p $(BIN_DIR)
	curl -fsSL "$(FFMPEG_WIN_AMD64)" -o /tmp/ffmpeg-win.zip
	unzip -jq /tmp/ffmpeg-win.zip '*/bin/ffmpeg.exe' -d $(BIN_DIR)/
	mv $(BIN_DIR)/ffmpeg.exe $(BIN_DIR)/ffmpeg  # embed.go uses bare name; extract.go adds .exe
	@echo "Note: exiftool for Windows must be placed manually at $(BIN_DIR)/exiftool"

# ── fetch all platforms (for CI cross-build) ─────────────────────────────────
bundle-all: bundle-linux-amd64

# ── cross-compile release binaries ───────────────────────────────────────────
release: bundle-all
	GOOS=linux  GOARCH=amd64 go build -tags with_bundled_tools -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64   .
	GOOS=darwin GOARCH=amd64 go build -tags with_bundled_tools -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64  .
	GOOS=darwin GOARCH=arm64 go build -tags with_bundled_tools -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64  .
	GOOS=windows GOARCH=amd64 go build -tags with_bundled_tools -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe .

# ── clean ─────────────────────────────────────────────────────────────────────
clean:
	rm -f $(BINARY)
	rm -rf dist/
	rm -f $(BIN_DIR)/exiftool $(BIN_DIR)/ffmpeg $(BIN_DIR)/ffmpeg.exe
