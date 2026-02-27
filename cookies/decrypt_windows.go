//go:build windows

package cookies

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GetChromeAESKey reads Chrome's App-Bound Encryption AES-256 key from
// the Local State file, decrypts it via DPAPI, and returns the raw 32-byte key.
// This is required to decrypt cookies with the "v20" prefix (Chrome ≥ 127).
func GetChromeAESKey() ([]byte, error) {
	statePath := ChromeLocalStatePath()
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("read local state: %w", err)
	}

	var localState struct {
		OSCrypt struct {
			AppBoundEncryptedKey string `json:"app_bound_encrypted_key"`
			EncryptedKey         string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(data, &localState); err != nil {
		return nil, fmt.Errorf("parse local state: %w", err)
	}

	// Try App-Bound key first (v20, Chrome ≥ 127)
	if localState.OSCrypt.AppBoundEncryptedKey != "" {
		key, err := decryptAppBoundKey(localState.OSCrypt.AppBoundEncryptedKey)
		if err == nil {
			return key, nil
		}
		// fall through to DPAPI-only key
	}

	// Legacy DPAPI key (v10/v11 on Windows uses the DPAPI-encrypted key)
	if localState.OSCrypt.EncryptedKey == "" {
		return nil, fmt.Errorf("no encrypted key in Local State")
	}
	return decryptDPAPIKey(localState.OSCrypt.EncryptedKey)
}

// decryptAppBoundKey decrypts the APPB-prefixed App-Bound key blob.
func decryptAppBoundKey(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 app_bound_encrypted_key: %w", err)
	}
	// Strip "APPB" prefix (4 bytes)
	if len(raw) < 4 || string(raw[:4]) != "APPB" {
		return nil, fmt.Errorf("unexpected app_bound_encrypted_key prefix")
	}
	return dpapiDecrypt(raw[4:])
}

// decryptDPAPIKey decodes the base64 key (strips "DPAPI" prefix) and decrypts via DPAPI.
func decryptDPAPIKey(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 encrypted_key: %w", err)
	}
	if strings.HasPrefix(string(raw), "DPAPI") {
		raw = raw[5:]
	}
	return dpapiDecrypt(raw)
}

// dpapiDecrypt calls Windows CryptUnprotectData on the given blob.
func dpapiDecrypt(data []byte) ([]byte, error) {
	var outBlob windows.DataBlob
	inBlob := windows.DataBlob{
		Size: uint32(len(data)),
		Data: &data[0],
	}

	err := windows.CryptUnprotectData(&inBlob, nil, nil, uintptr(unsafe.Pointer(nil)), nil, 0, &outBlob)
	if err != nil {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))

	result := make([]byte, outBlob.Size)
	copy(result, unsafe.Slice(outBlob.Data, outBlob.Size))
	return result, nil
}

// DecryptV20Cookie decrypts a Chrome v20 cookie (App-Bound AES-256-GCM).
// Format: "v20" + 12-byte nonce + ciphertext + 16-byte tag (tag is part of GCM output).
func DecryptV20Cookie(enc []byte, aesKey []byte) (string, error) {
	if len(enc) < 3+12+16 {
		return "", fmt.Errorf("v20 blob too short: %d bytes", len(enc))
	}
	// Skip "v20" prefix
	payload := enc[3:]
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
		return "", fmt.Errorf("gcm decrypt: %w", err)
	}
	return string(plaintext), nil
}
