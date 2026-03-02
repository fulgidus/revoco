// Package metadata defines types for Gmail Takeout data.
package metadata

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// EmailMessage represents a parsed email message from MBOX.
type EmailMessage struct {
	From           string            `json:"from"`
	To             []string          `json:"to"`
	CC             []string          `json:"cc"`
	BCC            []string          `json:"bcc"`
	Subject        string            `json:"subject"`
	Date           time.Time         `json:"date"`
	MessageID      string            `json:"message_id"`
	InReplyTo      string            `json:"in_reply_to"`
	ContentType    string            `json:"content_type"`
	Labels         []string          `json:"labels"`
	HasAttachments bool              `json:"has_attachments"`
	Headers        map[string]string `json:"headers"`
	BodyPreview    string            `json:"body_preview,omitempty"` // First 200 chars
}

// MboxFile represents an MBOX file metadata.
type MboxFile struct {
	Path         string `json:"path"`
	Label        string `json:"label"`
	MessageCount int    `json:"message_count"`
}

// GmailLibrary holds all parsed Gmail data.
type GmailLibrary struct {
	Messages  []EmailMessage `json:"messages"`
	MboxFiles []MboxFile     `json:"mbox_files"`
}

// ParseMboxHeader parses an email message header string into EmailMessage.
// Uses net/mail.ReadMessage to parse RFC 822 headers.
func ParseMboxHeader(headerData string) (EmailMessage, error) {
	msg, err := mail.ReadMessage(strings.NewReader(headerData))
	if err != nil {
		return EmailMessage{}, fmt.Errorf("parse email: %w", err)
	}

	em := EmailMessage{
		Headers: make(map[string]string),
	}

	// Extract standard headers
	em.From = msg.Header.Get("From")
	em.Subject = msg.Header.Get("Subject")
	em.MessageID = msg.Header.Get("Message-ID")
	em.InReplyTo = msg.Header.Get("In-Reply-To")
	em.ContentType = msg.Header.Get("Content-Type")

	// Parse date
	dateStr := msg.Header.Get("Date")
	if dateStr != "" {
		em.Date, _ = mail.ParseDate(dateStr)
	}

	// Parse recipients (To, CC, BCC)
	em.To = parseAddressList(msg.Header.Get("To"))
	em.CC = parseAddressList(msg.Header.Get("Cc"))
	em.BCC = parseAddressList(msg.Header.Get("Bcc"))

	// Gmail-specific: Extract labels
	labelsStr := msg.Header.Get("X-Gmail-Labels")
	if labelsStr != "" {
		em.Labels = parseGmailLabels(labelsStr)
	}

	// Check for attachments based on Content-Type
	ct := strings.ToLower(em.ContentType)
	if strings.Contains(ct, "multipart/mixed") || strings.Contains(ct, "multipart/related") {
		em.HasAttachments = true
	}

	// Store all headers for debugging/future use
	for k := range msg.Header {
		em.Headers[k] = msg.Header.Get(k)
	}

	return em, nil
}

// parseAddressList parses comma-separated email addresses.
func parseAddressList(addrStr string) []string {
	if addrStr == "" {
		return nil
	}

	// Use mail.ParseAddressList for RFC 822 address parsing
	addresses, err := mail.ParseAddressList(addrStr)
	if err != nil {
		// Fallback: split by comma
		parts := strings.Split(addrStr, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}

	result := make([]string, len(addresses))
	for i, addr := range addresses {
		// Use Address field to get just the email, or String() for full format
		if addr.Address != "" {
			result[i] = addr.Address
		} else {
			result[i] = addr.String()
		}
	}
	return result
}

// parseGmailLabels parses Gmail's X-Gmail-Labels header.
// Format: "Label1,Label2,Label3"
func parseGmailLabels(labelsStr string) []string {
	parts := strings.Split(labelsStr, ",")
	labels := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			labels = append(labels, trimmed)
		}
	}
	return labels
}

// ToCSVRow converts EmailMessage to CSV row values.
func (em *EmailMessage) ToCSVRow() []string {
	return []string{
		em.MessageID,
		em.From,
		strings.Join(em.To, "; "),
		em.Subject,
		em.Date.Format(time.RFC3339),
		strings.Join(em.Labels, "; "),
		fmt.Sprintf("%v", em.HasAttachments),
	}
}

// CSVHeaders returns the CSV column headers for email exports.
func CSVHeaders() []string {
	return []string{
		"Message-ID",
		"From",
		"To",
		"Subject",
		"Date",
		"Labels",
		"Has Attachments",
	}
}
