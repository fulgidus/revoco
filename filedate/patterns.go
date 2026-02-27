// Package filedate extracts dates from filenames using patterns common to
// Google Photos exports (Pixel, Samsung, WhatsApp, Telegram, Facebook, etc.).
package filedate

import (
	"regexp"
	"strconv"
	"time"
)

// Pattern pairs a compiled regexp with an extraction function.
type Pattern struct {
	Name string
	Re   *regexp.Regexp
	// Extract returns a time.Time from named subgroups of a regexp match.
	// Returns zero Time if the match cannot be parsed.
	Extract func(m []string) time.Time
}

// dateFromParts is a helper that assembles a time.Time from string fields.
// month is 1-based. Returns zero Time on any parse error.
func dateFromParts(year, month, day, hour, min, sec string) time.Time {
	y, err := strconv.Atoi(year)
	if err != nil {
		return time.Time{}
	}
	mo, err := strconv.Atoi(month)
	if err != nil {
		return time.Time{}
	}
	d, err := strconv.Atoi(day)
	if err != nil {
		return time.Time{}
	}
	h, _ := strconv.Atoi(hour)
	m, _ := strconv.Atoi(min)
	s, _ := strconv.Atoi(sec)
	return time.Date(y, time.Month(mo), d, h, m, s, 0, time.Local)
}

// Patterns is the ordered list of filename date patterns, most specific first.
var Patterns = []Pattern{
	{
		// PXL_20250131_200756791.jpg  (Pixel camera)
		Name: "PXL",
		Re:   regexp.MustCompile(`(?i)^PXL_(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// IMG_20220616_081525.jpg
		Name: "IMG_",
		Re:   regexp.MustCompile(`(?i)^IMG_(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// IMG20240415142704.jpg (no underscore)
		Name: "IMGcompact",
		Re:   regexp.MustCompile(`(?i)^IMG(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// VID_20171001_171501.mp4
		Name: "VID_",
		Re:   regexp.MustCompile(`(?i)^VID_(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// Screenshot_20240107-121214.png
		Name: "Screenshot_",
		Re:   regexp.MustCompile(`(?i)^Screenshot_(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// Screenshot_2022-02-03-13-35-50-44_....jpg
		Name: "Screenshot_dash",
		Re:   regexp.MustCompile(`(?i)^Screenshot_(\d{4})-(\d{2})-(\d{2})-(\d{2})-(\d{2})-(\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// screen-20241203-190614.mp4
		Name: "screen-",
		Re:   regexp.MustCompile(`(?i)^screen-(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// IMG-20240501-WA0002.jpg  (WhatsApp — no time, use noon)
		Name: "WhatsApp",
		Re:   regexp.MustCompile(`(?i)^IMG-(\d{4})(\d{2})(\d{2})-WA`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], "12", "0", "0")
		},
	},
	{
		// photo_2024-03-11 16.18.18.jpeg  (Telegram)
		Name: "Telegram",
		Re:   regexp.MustCompile(`(?i)^photo_(\d{4})-(\d{2})-(\d{2})[ _](\d{2})[._](\d{2})[._](\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// 20241019_093606.jpg
		Name: "YYYYMMDD_HHMMSS",
		Re:   regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// 2024-05-27 12.21.39.mp4  or  2024-05-27_12.21.39
		Name: "YYYY-MM-DD",
		Re:   regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})[ _](\d{2})[._](\d{2})[._](\d{2})`),
		Extract: func(m []string) time.Time {
			return dateFromParts(m[1], m[2], m[3], m[4], m[5], m[6])
		},
	},
	{
		// FB_IMG_1540569731664.jpg  (Facebook — Unix milliseconds)
		Name: "Facebook",
		Re:   regexp.MustCompile(`(?i)^FB_IMG_(\d{10,13})`),
		Extract: func(m []string) time.Time {
			ms, err := strconv.ParseInt(m[1], 10, 64)
			if err != nil {
				return time.Time{}
			}
			// If 13 digits, it's milliseconds; if 10, it's seconds
			if ms > 1e12 {
				ms /= 1000
			}
			return time.Unix(ms, 0).Local()
		},
	},
}

// Extract attempts to parse a date from the given filename (basename only).
// Returns the matched time and pattern name, or zero Time if nothing matched.
func Extract(filename string) (time.Time, string) {
	for _, p := range Patterns {
		m := p.Re.FindStringSubmatch(filename)
		if m == nil {
			continue
		}
		t := p.Extract(m)
		if t.IsZero() {
			continue
		}
		return t, p.Name
	}
	return time.Time{}, ""
}
