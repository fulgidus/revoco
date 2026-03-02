// Package metadata defines types for Google Contacts Takeout data.
package metadata

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"mime/quotedprintable"
	"strings"
	"time"
)

// Contact represents a parsed contact from vCard format.
type Contact struct {
	UID          string         `json:"uid"`
	FullName     string         `json:"full_name"`
	GivenName    string         `json:"given_name"`
	FamilyName   string         `json:"family_name"`
	MiddleName   string         `json:"middle_name"`
	Prefix       string         `json:"prefix"`
	Suffix       string         `json:"suffix"`
	Nickname     string         `json:"nickname"`
	Emails       []EmailEntry   `json:"emails"`
	Phones       []PhoneEntry   `json:"phones"`
	Addresses    []AddressEntry `json:"addresses"`
	Organization string         `json:"organization"`
	Title        string         `json:"title"`
	Birthday     *time.Time     `json:"birthday,omitempty"`
	Notes        string         `json:"notes"`
	Photo        string         `json:"photo,omitempty"` // Base64-encoded image data
	Groups       []string       `json:"groups"`
	URL          string         `json:"url"`
	Version      string         `json:"version"` // vCard version (2.1, 3.0, 4.0)
}

// EmailEntry represents an email address with type.
type EmailEntry struct {
	Address string   `json:"address"`
	Type    []string `json:"type"` // HOME, WORK, INTERNET, etc.
	Primary bool     `json:"primary"`
}

// PhoneEntry represents a phone number with type.
type PhoneEntry struct {
	Number  string   `json:"number"`
	Type    []string `json:"type"` // HOME, WORK, CELL, etc.
	Primary bool     `json:"primary"`
}

// AddressEntry represents a physical address with type.
type AddressEntry struct {
	Street     string   `json:"street"`
	City       string   `json:"city"`
	Region     string   `json:"region"` // State/Province
	PostalCode string   `json:"postal_code"`
	Country    string   `json:"country"`
	Type       []string `json:"type"` // HOME, WORK, etc.
	Primary    bool     `json:"primary"`
}

// ContactsLibrary holds all parsed contacts data.
type ContactsLibrary struct {
	Contacts []Contact `json:"contacts"`
}

// ParseVCard parses vCard data from a reader and returns all contacts found.
// Supports vCard versions 2.1, 3.0, and 4.0 (RFC 2426, RFC 6350).
func ParseVCard(reader io.Reader) ([]Contact, error) {
	scanner := bufio.NewScanner(reader)
	var currentVCard []string
	var contacts []Contact
	var inCard bool

	for scanner.Scan() {
		line := scanner.Text()

		// Handle line folding (RFC 6350 section 3.2)
		// Lines starting with space or tab continue the previous line
		if len(currentVCard) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			// Append continuation to previous line (remove leading whitespace)
			lastIdx := len(currentVCard) - 1
			currentVCard[lastIdx] += strings.TrimPrefix(strings.TrimPrefix(line, " "), "\t")
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "BEGIN:VCARD") {
			inCard = true
			currentVCard = []string{}
			continue
		}

		if strings.HasPrefix(trimmed, "END:VCARD") {
			if inCard && len(currentVCard) > 0 {
				contact, err := parseVCardBlock(currentVCard)
				if err == nil {
					contacts = append(contacts, contact)
				}
			}
			inCard = false
			currentVCard = []string{}
			continue
		}

		if inCard {
			currentVCard = append(currentVCard, trimmed)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan vcard: %w", err)
	}

	return contacts, nil
}

// parseVCardBlock parses a single vCard block into a Contact.
func parseVCardBlock(lines []string) (Contact, error) {
	contact := Contact{
		Emails:    []EmailEntry{},
		Phones:    []PhoneEntry{},
		Addresses: []AddressEntry{},
		Groups:    []string{},
	}

	for _, line := range lines {
		// Split property name and value
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		propName := line[:colonIdx]
		propValue := line[colonIdx+1:]

		// Parse property parameters (e.g., "EMAIL;TYPE=HOME:...")
		params := make(map[string]string)
		baseProp := propName
		if semiIdx := strings.Index(propName, ";"); semiIdx != -1 {
			baseProp = propName[:semiIdx]
			paramStr := propName[semiIdx+1:]
			params = parseParams(paramStr)
		}

		// Decode value if needed
		propValue = decodeValue(propValue, params)

		// Handle property
		switch strings.ToUpper(baseProp) {
		case "VERSION":
			contact.Version = propValue

		case "UID":
			contact.UID = propValue

		case "FN":
			contact.FullName = propValue

		case "N":
			// Format: FamilyName;GivenName;MiddleName;Prefix;Suffix
			parts := strings.Split(propValue, ";")
			if len(parts) > 0 {
				contact.FamilyName = parts[0]
			}
			if len(parts) > 1 {
				contact.GivenName = parts[1]
			}
			if len(parts) > 2 {
				contact.MiddleName = parts[2]
			}
			if len(parts) > 3 {
				contact.Prefix = parts[3]
			}
			if len(parts) > 4 {
				contact.Suffix = parts[4]
			}

		case "NICKNAME":
			contact.Nickname = propValue

		case "EMAIL":
			types := parseTypes(params)
			primary := contains(types, "PREF") || contains(types, "PRIMARY")
			contact.Emails = append(contact.Emails, EmailEntry{
				Address: propValue,
				Type:    types,
				Primary: primary,
			})

		case "TEL":
			types := parseTypes(params)
			primary := contains(types, "PREF") || contains(types, "PRIMARY")
			contact.Phones = append(contact.Phones, PhoneEntry{
				Number:  propValue,
				Type:    types,
				Primary: primary,
			})

		case "ADR":
			// Format: POBox;Extended;Street;City;Region;PostalCode;Country
			parts := strings.Split(propValue, ";")
			addr := AddressEntry{
				Type:    parseTypes(params),
				Primary: contains(parseTypes(params), "PREF"),
			}
			if len(parts) > 2 {
				addr.Street = parts[2]
			}
			if len(parts) > 3 {
				addr.City = parts[3]
			}
			if len(parts) > 4 {
				addr.Region = parts[4]
			}
			if len(parts) > 5 {
				addr.PostalCode = parts[5]
			}
			if len(parts) > 6 {
				addr.Country = parts[6]
			}
			contact.Addresses = append(contact.Addresses, addr)

		case "ORG":
			contact.Organization = propValue

		case "TITLE":
			contact.Title = propValue

		case "BDAY":
			// Try parsing various formats
			if bd := parseBirthday(propValue); bd != nil {
				contact.Birthday = bd
			}

		case "NOTE":
			contact.Notes = propValue

		case "PHOTO":
			// Store base64-encoded photo data
			if encoding, ok := params["ENCODING"]; ok && strings.ToUpper(encoding) == "BASE64" {
				// Remove whitespace from base64 data
				contact.Photo = strings.ReplaceAll(propValue, " ", "")
			} else {
				contact.Photo = propValue
			}

		case "CATEGORIES":
			contact.Groups = strings.Split(propValue, ",")

		case "URL":
			contact.URL = propValue
		}
	}

	return contact, nil
}

// parseParams parses vCard property parameters (e.g., "TYPE=HOME;ENCODING=QUOTED-PRINTABLE").
func parseParams(paramStr string) map[string]string {
	params := make(map[string]string)
	parts := strings.Split(paramStr, ";")
	for _, part := range parts {
		if eqIdx := strings.Index(part, "="); eqIdx != -1 {
			key := strings.ToUpper(strings.TrimSpace(part[:eqIdx]))
			value := strings.TrimSpace(part[eqIdx+1:])
			// Remove quotes if present
			value = strings.Trim(value, "\"")
			params[key] = value
		} else {
			// Handle shorthand TYPE notation (e.g., "HOME" instead of "TYPE=HOME")
			params["TYPE"] = strings.ToUpper(strings.TrimSpace(part))
		}
	}
	return params
}

// decodeValue decodes a property value based on encoding/charset parameters.
func decodeValue(value string, params map[string]string) string {
	// Handle QUOTED-PRINTABLE encoding (vCard 2.1/3.0)
	if encoding, ok := params["ENCODING"]; ok {
		switch strings.ToUpper(encoding) {
		case "QUOTED-PRINTABLE", "QUOTED PRINTABLE":
			reader := quotedprintable.NewReader(strings.NewReader(value))
			decoded, err := io.ReadAll(reader)
			if err == nil {
				value = string(decoded)
			}
		case "BASE64", "B":
			// Base64 decoding for inline data
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err == nil {
				value = string(decoded)
			}
		}
	}

	// Handle CHARSET parameter (mostly for legacy vCard 2.1)
	if charset, ok := params["CHARSET"]; ok {
		// Most modern systems use UTF-8, but we acknowledge the parameter
		_ = charset // For future charset conversion if needed
	}

	return value
}

// parseTypes extracts TYPE parameter values from property parameters.
func parseTypes(params map[string]string) []string {
	var types []string
	if typeVal, ok := params["TYPE"]; ok {
		// Handle comma-separated types (vCard 4.0) or single type
		for _, t := range strings.Split(typeVal, ",") {
			trimmed := strings.TrimSpace(strings.ToUpper(t))
			if trimmed != "" {
				types = append(types, trimmed)
			}
		}
	}
	return types
}

// parseBirthday attempts to parse birthday from various formats.
func parseBirthday(value string) *time.Time {
	// Common formats: YYYY-MM-DD, YYYYMMDD, --MMDD, etc.
	formats := []string{
		"2006-01-02",
		"20060102",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return &t
		}
	}

	// Partial date without year (--MM-DD)
	if strings.HasPrefix(value, "--") {
		value = "1900" + value[1:] // Use placeholder year
		if t, err := time.Parse("2006-01-02", value); err == nil {
			return &t
		}
	}

	return nil
}

// contains checks if a slice contains a string (case-insensitive).
func contains(slice []string, item string) bool {
	upper := strings.ToUpper(item)
	for _, s := range slice {
		if strings.ToUpper(s) == upper {
			return true
		}
	}
	return false
}

// ToCSVRow converts Contact to CSV row values.
func (c *Contact) ToCSVRow() []string {
	email := ""
	if len(c.Emails) > 0 {
		email = c.Emails[0].Address
	}

	phone := ""
	if len(c.Phones) > 0 {
		phone = c.Phones[0].Number
	}

	birthday := ""
	if c.Birthday != nil {
		birthday = c.Birthday.Format("2006-01-02")
	}

	return []string{
		c.UID,
		c.FullName,
		c.GivenName,
		c.FamilyName,
		email,
		phone,
		c.Organization,
		c.Title,
		birthday,
		c.Notes,
	}
}

// CSVHeaders returns the CSV column headers for contact exports.
func CSVHeaders() []string {
	return []string{
		"UID",
		"Full Name",
		"Given Name",
		"Family Name",
		"Email",
		"Phone",
		"Organization",
		"Title",
		"Birthday",
		"Notes",
	}
}
