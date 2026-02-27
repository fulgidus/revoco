// Package cookies provides Chrome cookie decryption and Netscape jar I/O.
//
// Supports Chrome v10 (hardcoded "peanuts" key) and v11 (KWallet / user-supplied
// password) AES-128-CBC encryption on Linux.
//
// Key derivation (both versions):
//
//	key = PBKDF2-HMAC-SHA1(password, "saltysalt", 1 iteration, 16 bytes)
//	IV  = 16 × 0x20 (space character)
//
// v11 ciphertext starts with the 3-byte prefix "v11"; after decryption the first
// 32 bytes of plaintext are a per-scope header that must be stripped.
// v10 ciphertext starts with "v10"; no header strip required.
// Anything else is returned as plain UTF-8 text.
package cookies

import (
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"time"

	"crypto/sha1"
	"golang.org/x/crypto/pbkdf2"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const (
	chromeSalt   = "saltysalt"
	chromeIter   = 1
	chromeKeyLen = 16
	v10Password  = "peanuts" // hardcoded for v10
)

// chromeIV is 16 space characters (0x20).
var chromeIV = func() []byte {
	b := make([]byte, 16)
	for i := range b {
		b[i] = 0x20
	}
	return b
}()

// chromeEpochOffset converts Chrome's internal timestamps (microseconds since
// 1601-01-01) to Unix seconds.
const chromeEpochOffset = int64(11644473600)

// CookieRow holds one row from the Chrome cookies SQLite table.
type CookieRow struct {
	Host      string
	Name      string
	Value     string // plaintext after decryption
	Path      string
	ExpiresAt time.Time
	Secure    bool
	HTTPOnly  bool
}

// ChromeDecryptor can decrypt Chrome cookies using the v10/v11 scheme on
// Linux/macOS, and the v20 AES-256-GCM scheme on Windows (Chrome ≥ 127).
type ChromeDecryptor struct {
	keyV10 []byte
	keyV11 []byte
	keyV20 []byte // Windows App-Bound AES-256 key; nil on other platforms
}

// NewChromeDecryptor creates a decryptor for v10/v11 (PBKDF2-based) cookies.
// password is the v11 key material (e.g. from KWallet / macOS Keychain).
// Pass "" to use an empty password for v11 (Chrome default on many Linux setups).
func NewChromeDecryptor(password string) *ChromeDecryptor {
	return &ChromeDecryptor{
		keyV10: deriveKey(v10Password),
		keyV11: deriveKey(password),
	}
}

// NewChromeDecryptorFromKey creates a decryptor with a raw AES key (Windows v20).
// The keyV10/keyV11 fields are populated with the peanuts / empty-password defaults
// as a fallback for any v10/v11 cookies that may still be present.
func NewChromeDecryptorFromKey(aesKey []byte) *ChromeDecryptor {
	return &ChromeDecryptor{
		keyV10: deriveKey(v10Password),
		keyV11: deriveKey(""),
		keyV20: aesKey,
	}
}

func deriveKey(password string) []byte {
	return pbkdf2.Key([]byte(password), []byte(chromeSalt), chromeIter, chromeKeyLen, sha1.New)
}

// Decrypt decrypts an encrypted_value blob from Chrome's cookies table.
// Handles v10, v11 (PBKDF2 AES-128-CBC) and v20 (App-Bound AES-256-GCM, Windows only).
func (d *ChromeDecryptor) Decrypt(enc []byte) (string, error) {
	if len(enc) == 0 {
		return "", nil
	}
	if len(enc) < 3 {
		return string(enc), nil
	}

	prefix := string(enc[:3])
	switch prefix {
	case "v20":
		if d.keyV20 != nil {
			return decryptV20(enc, d.keyV20)
		}
		return "", fmt.Errorf("v20 cookie encountered but no AES-256 key available (Windows only)")
	case "v11":
		val, err := d.decryptAES(enc[3:], d.keyV11, 32)
		if err != nil {
			// Fallback: try empty password
			emptyKey := deriveKey("")
			val2, err2 := d.decryptAES(enc[3:], emptyKey, 32)
			if err2 != nil {
				return "", fmt.Errorf("v11 decrypt failed: %w (empty-key: %v)", err, err2)
			}
			return val2, nil
		}
		return val, nil
	case "v10":
		return d.decryptAES(enc[3:], d.keyV10, 0)
	default:
		return string(enc), nil
	}
}

// decryptV20 decrypts a v20 (AES-256-GCM) cookie blob.
// Format after stripping "v20": 12-byte nonce + ciphertext+tag.
func decryptV20(enc []byte, aesKey []byte) (string, error) {
	payload := enc[3:] // strip "v20"
	if len(payload) < 12+16 {
		return "", fmt.Errorf("v20 blob too short")
	}
	nonce := payload[:12]
	ciphertext := payload[12:]

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("aes-gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("gcm open: %w", err)
	}
	return string(plaintext), nil
}

// decryptAES performs AES-128-CBC decryption with manual PKCS7 unpadding,
// then strips skipBytes from the start of the plaintext.
func (d *ChromeDecryptor) decryptAES(ciphertext, key []byte, skipBytes int) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("ciphertext length %d is not a multiple of block size", len(ciphertext))
	}
	if len(ciphertext) == 0 {
		return "", nil
	}

	mode := cipher.NewCBCDecrypter(block, chromeIV)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding manually (Chrome sets AutoPadding=false in Node.js)
	if len(plaintext) > 0 {
		padLen := int(plaintext[len(plaintext)-1])
		if padLen > 0 && padLen <= aes.BlockSize && padLen <= len(plaintext) {
			plaintext = plaintext[:len(plaintext)-padLen]
		}
	}

	if skipBytes > 0 && len(plaintext) > skipBytes {
		plaintext = plaintext[skipBytes:]
	}

	return string(plaintext), nil
}

// ReadChromeCookies opens the Chrome SQLite cookies DB, decrypts the values for
// the specified domains, and returns the rows.
func ReadChromeCookies(dbPath string, domains []string, dec *ChromeDecryptor) ([]CookieRow, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_journal=wal")
	if err != nil {
		return nil, fmt.Errorf("open chrome db: %w", err)
	}
	defer db.Close()

	placeholders := make([]string, len(domains))
	args := make([]interface{}, len(domains))
	for i, d := range domains {
		placeholders[i] = "?"
		args[i] = d
	}

	query := fmt.Sprintf(
		`SELECT host_key, name, encrypted_value, path, expires_utc, is_secure, is_httponly
		 FROM cookies
		 WHERE host_key IN (%s)
		 ORDER BY host_key, name`,
		joinStrings(placeholders, ","),
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query cookies: %w", err)
	}
	defer rows.Close()

	var result []CookieRow
	for rows.Next() {
		var (
			host, name, path string
			encVal           []byte
			expiresUs        int64
			isSecure, isHTTP int
		)
		if err := rows.Scan(&host, &name, &encVal, &path, &expiresUs, &isSecure, &isHTTP); err != nil {
			continue
		}
		val, err := dec.Decrypt(encVal)
		if err != nil || val == "" {
			continue
		}
		var expiresAt time.Time
		if expiresUs > 0 {
			unixSec := expiresUs/1_000_000 - chromeEpochOffset
			expiresAt = time.Unix(unixSec, 0)
		}
		result = append(result, CookieRow{
			Host:      host,
			Name:      name,
			Value:     val,
			Path:      path,
			ExpiresAt: expiresAt,
			Secure:    isSecure != 0,
			HTTPOnly:  isHTTP != 0,
		})
	}
	return result, rows.Err()
}

// BuildHTTPJar converts CookieRows into a stdlib http.CookieJar.
func BuildHTTPJar(rows []CookieRow) (http.CookieJar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	// Group cookies by scheme+host
	byHost := make(map[string][]*http.Cookie)
	for _, r := range rows {
		host := r.Host
		scheme := "https"
		if !r.Secure {
			scheme = "http"
		}
		key := scheme + "://" + host
		c := &http.Cookie{
			Name:     r.Name,
			Value:    r.Value,
			Path:     r.Path,
			Domain:   r.Host,
			Expires:  r.ExpiresAt,
			Secure:   r.Secure,
			HttpOnly: r.HTTPOnly,
		}
		byHost[key] = append(byHost[key], c)
	}
	for rawURL, cookies := range byHost {
		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		jar.SetCookies(u, cookies)
	}
	return jar, nil
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

// DefaultChromeDBPath returns the default Chrome cookies database path for the
// current platform. Returns an error if the path does not exist.
func DefaultChromeDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home dir: %w", err)
	}

	candidates := defaultChromeCandidates(home)

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("Chrome cookies DB not found; tried: %v", candidates)
}

// ExtractToJar extracts Google-domain cookies from Chrome's SQLite DB,
// decrypts them with the given v11 password, and writes a Netscape cookie
// jar to jarPath. Returns the number of cookies extracted.
func ExtractToJar(chromeDBPath, v11Password, jarPath string) (int, error) {
	dec := NewChromeDecryptor(v11Password)
	rows, err := ReadChromeCookies(chromeDBPath, GoogleDomains, dec)
	if err != nil {
		return 0, fmt.Errorf("read chrome cookies: %w", err)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("no Google cookies found in Chrome DB")
	}
	if err := WriteNetscapeJar(jarPath, rows); err != nil {
		return 0, fmt.Errorf("write cookie jar: %w", err)
	}
	return len(rows), nil
}
