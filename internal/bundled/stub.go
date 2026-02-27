//go:build !with_bundled_tools

// Package bundled exposes the embedded binary blobs for exiftool and ffmpeg.
// In dev builds (without the with_bundled_tools tag), all blobs are nil and
// the runtime falls back to PATH lookup.
package bundled

// ExifTool is the embedded exiftool binary for the current platform.
// Nil when built without -tags with_bundled_tools.
var ExifTool []byte

// FFmpeg is the embedded ffmpeg binary for the current platform.
// Nil when built without -tags with_bundled_tools.
var FFmpeg []byte
