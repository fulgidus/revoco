package metadata

import (
	"strings"
	"testing"
	"time"
)

func TestParseMboxHeader(t *testing.T) {
	// Synthetic email header
	header := `From: sender@example.com
To: recipient@example.com
Cc: cc@example.com
Subject: Test Email Subject
Date: Mon, 02 Jan 2006 15:04:05 -0700
Message-ID: <msg123@example.com>
Content-Type: text/plain; charset=UTF-8
X-Gmail-Labels: Inbox,Important

Body content starts here.
`

	msg, err := ParseMboxHeader(header)
	if err != nil {
		t.Fatalf("ParseMboxHeader failed: %v", err)
	}

	if msg.From != "sender@example.com" {
		t.Errorf("From = %q, want %q", msg.From, "sender@example.com")
	}

	if len(msg.To) != 1 || msg.To[0] != "recipient@example.com" {
		t.Errorf("To = %v, want [recipient@example.com]", msg.To)
	}

	if len(msg.CC) != 1 || msg.CC[0] != "cc@example.com" {
		t.Errorf("CC = %v, want [cc@example.com]", msg.CC)
	}

	if msg.Subject != "Test Email Subject" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "Test Email Subject")
	}

	if msg.MessageID != "<msg123@example.com>" {
		t.Errorf("MessageID = %q, want %q", msg.MessageID, "<msg123@example.com>")
	}

	if len(msg.Labels) != 2 {
		t.Errorf("Labels count = %d, want 2", len(msg.Labels))
	}

	wantLabels := map[string]bool{"Inbox": true, "Important": true}
	for _, label := range msg.Labels {
		if !wantLabels[label] {
			t.Errorf("Unexpected label: %q", label)
		}
	}
}

func TestParseMultiRecipients(t *testing.T) {
	header := `From: sender@example.com
To: alice@example.com, bob@example.com, charlie@example.com
Cc: dave@example.com, eve@example.com
Bcc: admin@example.com
Subject: Multi-recipient test
Date: Mon, 02 Jan 2006 15:04:05 -0700
Message-ID: <multi@example.com>

Body
`

	msg, err := ParseMboxHeader(header)
	if err != nil {
		t.Fatalf("ParseMboxHeader failed: %v", err)
	}

	if len(msg.To) != 3 {
		t.Errorf("To count = %d, want 3", len(msg.To))
	}

	if len(msg.CC) != 2 {
		t.Errorf("CC count = %d, want 2", len(msg.CC))
	}

	if len(msg.BCC) != 1 {
		t.Errorf("BCC count = %d, want 1", len(msg.BCC))
	}
}

func TestParseGmailLabels(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"Inbox", []string{"Inbox"}},
		{"Inbox,Sent", []string{"Inbox", "Sent"}},
		{"Inbox, Sent, Important", []string{"Inbox", "Sent", "Important"}},
		{"", []string{}},
		{"  Label1  ,  Label2  ", []string{"Label1", "Label2"}},
	}

	for _, tt := range tests {
		got := parseGmailLabels(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseGmailLabels(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseGmailLabels(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestAttachmentDetection(t *testing.T) {
	tests := []struct {
		contentType string
		wantAttach  bool
	}{
		{"text/plain", false},
		{"text/html", false},
		{"multipart/mixed; boundary=abc", true},
		{"multipart/related; boundary=xyz", true},
		{"multipart/alternative", false},
	}

	for _, tt := range tests {
		header := "From: test@example.com\nContent-Type: " + tt.contentType + "\n\n"
		msg, err := ParseMboxHeader(header)
		if err != nil {
			t.Fatalf("ParseMboxHeader failed: %v", err)
		}

		if msg.HasAttachments != tt.wantAttach {
			t.Errorf("ContentType %q: HasAttachments = %v, want %v",
				tt.contentType, msg.HasAttachments, tt.wantAttach)
		}
	}
}

func TestParseAddressList(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"alice@example.com", 1},
		{"alice@example.com, bob@example.com", 2},
		{"Alice <alice@example.com>, Bob <bob@example.com>", 2},
		{"", 0},
	}

	for _, tt := range tests {
		got := parseAddressList(tt.input)
		if len(got) != tt.want {
			t.Errorf("parseAddressList(%q) count = %d, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestEmailMessageToCSVRow(t *testing.T) {
	msg := EmailMessage{
		MessageID:      "<test@example.com>",
		From:           "sender@example.com",
		To:             []string{"recipient@example.com"},
		Subject:        "Test Subject",
		Date:           time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC),
		Labels:         []string{"Inbox", "Important"},
		HasAttachments: true,
	}

	row := msg.ToCSVRow()
	if len(row) != 7 {
		t.Errorf("CSV row length = %d, want 7", len(row))
	}

	if row[0] != "<test@example.com>" {
		t.Errorf("CSV row[0] = %q, want message ID", row[0])
	}

	if !strings.Contains(row[5], "Inbox") {
		t.Errorf("CSV row[5] (labels) doesn't contain 'Inbox': %q", row[5])
	}

	if row[6] != "true" {
		t.Errorf("CSV row[6] (attachments) = %q, want 'true'", row[6])
	}
}

func TestCSVHeaders(t *testing.T) {
	headers := CSVHeaders()
	if len(headers) != 7 {
		t.Errorf("CSV headers length = %d, want 7", len(headers))
	}

	wantHeaders := []string{"Message-ID", "From", "To", "Subject", "Date", "Labels", "Has Attachments"}
	for i, want := range wantHeaders {
		if headers[i] != want {
			t.Errorf("Header[%d] = %q, want %q", i, headers[i], want)
		}
	}
}
