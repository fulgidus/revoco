//go:build with_bundled_tools

// Package bundled holds the platform-specific static binaries embedded at build
// time by the Makefile. Build with -tags with_bundled_tools to activate.
package bundled

import _ "embed"

//go:embed bin/exiftool
var ExifTool []byte

//go:embed bin/ffmpeg
var FFmpeg []byte
