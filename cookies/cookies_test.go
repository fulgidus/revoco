package cookies

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── Netscape jar round-trip ──────────────────────────────────────────────────

func TestWriteAndParseNetscapeJar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")

	rows := []CookieRow{
		{
			Host:      ".google.com",
			Name:      "SID",
			Value:     "abc123",
			Path:      "/",
			ExpiresAt: time.Unix(1700000000, 0),
			Secure:    true,
			HTTPOnly:  true,
		},
		{
			Host:      "photos.google.com",
			Name:      "NID",
			Value:     "xyz789",
			Path:      "/photos",
			ExpiresAt: time.Unix(1700001000, 0),
			Secure:    false,
			HTTPOnly:  false,
		},
	}

	if err := WriteNetscapeJar(path, rows); err != nil {
		t.Fatalf("WriteNetscapeJar: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("jar file not created: %v", err)
	}

	// Parse it back
	parsed, err := ParseNetscapeJar(path)
	if err != nil {
		t.Fatalf("ParseNetscapeJar: %v", err)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(parsed))
	}

	// First entry: .google.com SID (HttpOnly)
	l := parsed[0]
	if l.Domain != ".google.com" {
		t.Errorf("domain: got %q", l.Domain)
	}
	if l.Name != "SID" {
		t.Errorf("name: got %q", l.Name)
	}
	if l.Value != "abc123" {
		t.Errorf("value: got %q", l.Value)
	}
	if l.Path != "/" {
		t.Errorf("path: got %q", l.Path)
	}
	if !l.Secure {
		t.Error("expected secure=true")
	}
	if !l.HTTPOnly {
		t.Error("expected httponly=true")
	}
	if !l.Flag {
		t.Error("expected flag=true for domain starting with .")
	}
	if l.Expires != 1700000000 {
		t.Errorf("expires: got %d, want 1700000000", l.Expires)
	}

	// Second entry: photos.google.com NID
	l2 := parsed[1]
	if l2.Domain != "photos.google.com" {
		t.Errorf("domain: got %q", l2.Domain)
	}
	if l2.HTTPOnly {
		t.Error("expected httponly=false")
	}
	if l2.Secure {
		t.Error("expected secure=false")
	}
}

func TestParseNetscapeJarSkipsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")

	content := `# Netscape HTTP Cookie File
# This is a comment

.google.com	TRUE	/	TRUE	1700000000	SID	abc123
# Another comment
photos.google.com	FALSE	/	FALSE	0	NID	xyz
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseNetscapeJar(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 lines, got %d", len(parsed))
	}
}

func TestParseNetscapeJarMissingFile(t *testing.T) {
	_, err := ParseNetscapeJar("/nonexistent/cookies.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteNetscapeJarSessionCookie(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")

	rows := []CookieRow{
		{
			Host:  ".example.com",
			Name:  "session",
			Value: "tok",
			Path:  "/",
			// ExpiresAt is zero → session cookie
		},
	}

	if err := WriteNetscapeJar(path, rows); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseNetscapeJar(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 line, got %d", len(parsed))
	}
	if parsed[0].Expires != 0 {
		t.Errorf("session cookie should have 0 expires, got %d", parsed[0].Expires)
	}
}

// ── LoadNetscapeCookieJar ────────────────────────────────────────────────────

func TestLoadNetscapeCookieJar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")

	rows := []CookieRow{
		{
			Host:      ".google.com",
			Name:      "SID",
			Value:     "abc",
			Path:      "/",
			ExpiresAt: time.Now().Add(24 * time.Hour),
			Secure:    true,
		},
	}

	if err := WriteNetscapeJar(path, rows); err != nil {
		t.Fatal(err)
	}

	jar, err := LoadNetscapeCookieJar(path)
	if err != nil {
		t.Fatalf("LoadNetscapeCookieJar: %v", err)
	}
	if jar == nil {
		t.Fatal("jar is nil")
	}
}

// ── BuildHTTPJar ─────────────────────────────────────────────────────────────

func TestBuildHTTPJar(t *testing.T) {
	rows := []CookieRow{
		{
			Host:   ".google.com",
			Name:   "SID",
			Value:  "val",
			Path:   "/",
			Secure: true,
		},
	}

	jar, err := BuildHTTPJar(rows)
	if err != nil {
		t.Fatalf("BuildHTTPJar: %v", err)
	}
	if jar == nil {
		t.Fatal("jar is nil")
	}
}

// ── Chrome decryptor ─────────────────────────────────────────────────────────

func TestNewChromeDecryptor(t *testing.T) {
	dec := NewChromeDecryptor("")
	if dec == nil {
		t.Fatal("decryptor is nil")
	}
	if dec.keyV10 == nil {
		t.Error("keyV10 is nil")
	}
	if dec.keyV11 == nil {
		t.Error("keyV11 is nil")
	}
}

func TestDecryptEmptyBlob(t *testing.T) {
	dec := NewChromeDecryptor("")
	val, err := dec.Decrypt([]byte{})
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty, got %q", val)
	}
}

func TestDecryptPlaintextBlob(t *testing.T) {
	dec := NewChromeDecryptor("")
	// Anything that doesn't start with v10/v11/v20 is returned as-is
	val, err := dec.Decrypt([]byte("plain-cookie-value"))
	if err != nil {
		t.Fatalf("Decrypt plaintext: %v", err)
	}
	if val != "plain-cookie-value" {
		t.Errorf("got %q", val)
	}
}

func TestDecryptShortBlob(t *testing.T) {
	dec := NewChromeDecryptor("")
	val, err := dec.Decrypt([]byte("ab"))
	if err != nil {
		t.Fatalf("Decrypt short: %v", err)
	}
	if val != "ab" {
		t.Errorf("got %q", val)
	}
}

// ── DefaultChromeDBPath ──────────────────────────────────────────────────────

func TestDefaultChromeDBPathReturnsError(t *testing.T) {
	// On CI or systems without Chrome, this should return an error
	// rather than panicking
	_, err := DefaultChromeDBPath()
	// We don't check the result since Chrome may or may not be installed,
	// but it should not panic
	_ = err
}

// ── GoogleDomains ────────────────────────────────────────────────────────────

func TestGoogleDomainsNotEmpty(t *testing.T) {
	if len(GoogleDomains) == 0 {
		t.Error("GoogleDomains is empty")
	}
	for _, d := range GoogleDomains {
		if !strings.Contains(d, "google") {
			t.Errorf("unexpected domain %q in GoogleDomains", d)
		}
	}
}

// ── joinStrings ──────────────────────────────────────────────────────────────

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		parts []string
		sep   string
		want  string
	}{
		{nil, ",", ""},
		{[]string{"a"}, ",", "a"},
		{[]string{"a", "b", "c"}, ",", "a,b,c"},
		{[]string{"x", "y"}, " AND ", "x AND y"},
	}

	for _, tt := range tests {
		got := joinStrings(tt.parts, tt.sep)
		if got != tt.want {
			t.Errorf("joinStrings(%v, %q) = %q, want %q", tt.parts, tt.sep, got, tt.want)
		}
	}
}
