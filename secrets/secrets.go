// Package secrets provides password-encrypted storage for sensitive data
// (e.g. Chrome cookies, API tokens). Data is encrypted at rest with
// AES-256-GCM using an Argon2id-derived key. The plaintext never touches
// disk or shell history — passwords are read via raw terminal I/O.
//
// Storage layout:  ~/.revoco/secrets.json
//
//	{
//	  "salt":  "<base64>",   // 16-byte Argon2id salt
//	  "nonce": "<base64>",   // 12-byte GCM nonce
//	  "data":  "<base64>"    // AES-256-GCM ciphertext (JSON payload)
//	}
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
	"golang.org/x/term"
)

// Argon2id parameters (OWASP recommended minimums).
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32 // AES-256
	saltLen      = 16
	nonceLen     = 12
)

// envelope is the on-disk JSON structure.
type envelope struct {
	Salt  string `json:"salt"`
	Nonce string `json:"nonce"`
	Data  string `json:"data"`
}

// Payload is the decrypted in-memory container for all secrets.
// Keys are arbitrary labels (e.g. "google_cookies"), values are the
// sensitive strings.
type Payload map[string]string

// DefaultPath returns ~/.revoco/secrets.json.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("secrets: determine home dir: %w", err)
	}
	return filepath.Join(home, ".revoco", "secrets.json"), nil
}

// PromptPassword reads a password from the terminal without echo.
// It uses raw terminal mode so the password never appears in shell
// history or process listings.
func PromptPassword(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", fmt.Errorf("secrets: stdin is not a terminal — cannot read password securely")
	}
	fmt.Fprint(os.Stderr, prompt)
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr) // newline after hidden input
	if err != nil {
		return "", fmt.Errorf("secrets: read password: %w", err)
	}
	return string(pw), nil
}

// deriveKey runs Argon2id to produce a 256-bit key from password + salt.
func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

// Encrypt serialises payload as JSON, encrypts it with AES-256-GCM keyed by
// password, and writes the envelope to path.
func Encrypt(path string, password string, payload Payload) error {
	plain, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("secrets: marshal payload: %w", err)
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("secrets: generate salt: %w", err)
	}

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("secrets: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("secrets: gcm: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("secrets: generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plain, nil)

	env := envelope{
		Salt:  base64.StdEncoding.EncodeToString(salt),
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		Data:  base64.StdEncoding.EncodeToString(ciphertext),
	}

	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("secrets: marshal envelope: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("secrets: create dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("secrets: write file: %w", err)
	}

	// Wipe plaintext from memory
	for i := range plain {
		plain[i] = 0
	}
	return nil
}

// Decrypt reads the envelope from path, decrypts it with password, and
// returns the payload. Returns an error if the password is wrong or the
// file is corrupt.
func Decrypt(path string, password string) (Payload, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("secrets: read file: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("secrets: parse envelope: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode salt: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		return nil, fmt.Errorf("secrets: decode data: %w", err)
	}

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secrets: gcm: %w", err)
	}

	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("secrets: decrypt failed (wrong password?): %w", err)
	}

	var payload Payload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, fmt.Errorf("secrets: unmarshal payload: %w", err)
	}

	// Wipe plaintext from memory
	for i := range plain {
		plain[i] = 0
	}
	return payload, nil
}

// Exists reports whether the secrets file exists at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Store is a convenience wrapper that opens or creates the secrets file,
// sets one key, and re-encrypts. The password is prompted interactively
// if not supplied.
func Store(path, password, key, value string) error {
	var payload Payload
	if Exists(path) {
		var err error
		payload, err = Decrypt(path, password)
		if err != nil {
			return err
		}
	} else {
		payload = make(Payload)
	}
	payload[key] = value
	return Encrypt(path, password, payload)
}

// Get is a convenience wrapper that decrypts and returns a single key.
func Get(path, password, key string) (string, error) {
	payload, err := Decrypt(path, password)
	if err != nil {
		return "", err
	}
	val, ok := payload[key]
	if !ok {
		return "", fmt.Errorf("secrets: key %q not found", key)
	}
	return val, nil
}

// Delete removes a key from the secrets file and re-encrypts.
func Delete(path, password, key string) error {
	payload, err := Decrypt(path, password)
	if err != nil {
		return err
	}
	delete(payload, key)
	return Encrypt(path, password, payload)
}
