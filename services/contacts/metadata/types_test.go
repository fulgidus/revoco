package metadata

import (
	"strings"
	"testing"
	"time"
)

func TestParseVCard_SingleContact(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN:John Doe
N:Doe;John;Michael;;
EMAIL;TYPE=HOME:john.doe@example.com
TEL;TYPE=CELL:+1-555-1234
ORG:Acme Corp
TITLE:Software Engineer
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}

	c := contacts[0]
	if c.FullName != "John Doe" {
		t.Errorf("FullName = %q, want %q", c.FullName, "John Doe")
	}

	if c.GivenName != "John" {
		t.Errorf("GivenName = %q, want %q", c.GivenName, "John")
	}

	if c.FamilyName != "Doe" {
		t.Errorf("FamilyName = %q, want %q", c.FamilyName, "Doe")
	}

	if c.MiddleName != "Michael" {
		t.Errorf("MiddleName = %q, want %q", c.MiddleName, "Michael")
	}

	if len(c.Emails) != 1 {
		t.Fatalf("got %d emails, want 1", len(c.Emails))
	}

	if c.Emails[0].Address != "john.doe@example.com" {
		t.Errorf("Email = %q, want %q", c.Emails[0].Address, "john.doe@example.com")
	}

	if len(c.Phones) != 1 {
		t.Fatalf("got %d phones, want 1", len(c.Phones))
	}

	if c.Phones[0].Number != "+1-555-1234" {
		t.Errorf("Phone = %q, want %q", c.Phones[0].Number, "+1-555-1234")
	}

	if c.Organization != "Acme Corp" {
		t.Errorf("Organization = %q, want %q", c.Organization, "Acme Corp")
	}

	if c.Title != "Software Engineer" {
		t.Errorf("Title = %q, want %q", c.Title, "Software Engineer")
	}
}

func TestParseVCard_MultipleContacts(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN:Alice Smith
N:Smith;Alice;;;
EMAIL:alice@example.com
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:Bob Johnson
N:Johnson;Bob;;;
EMAIL:bob@example.com
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 2 {
		t.Fatalf("got %d contacts, want 2", len(contacts))
	}

	if contacts[0].FullName != "Alice Smith" {
		t.Errorf("Contact 0 FullName = %q, want %q", contacts[0].FullName, "Alice Smith")
	}

	if contacts[1].FullName != "Bob Johnson" {
		t.Errorf("Contact 1 FullName = %q, want %q", contacts[1].FullName, "Bob Johnson")
	}
}

func TestParseVCard_QuotedPrintable(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN;ENCODING=QUOTED-PRINTABLE:J=C3=BCrgen M=C3=BCller
N:M=C3=BCller;J=C3=BCrgen;;;
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}

	c := contacts[0]
	// Quoted-printable decoding should convert =C3=BC to ü
	if !strings.Contains(c.FullName, "rgen") {
		t.Errorf("FullName = %q, should contain decoded characters", c.FullName)
	}
}

func TestParseVCard_LineFolding(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN:Long Name That
 Wraps Across
 Multiple Lines
N:Lines;Multiple;;;
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}

	c := contacts[0]
	expected := "Long Name ThatWraps AcrossMultiple Lines"
	if c.FullName != expected {
		t.Errorf("FullName = %q, want %q (line folding)", c.FullName, expected)
	}
}

func TestParseVCard_EmptyInput(t *testing.T) {
	contacts, err := ParseVCard(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseVCard error on empty input: %v", err)
	}

	if len(contacts) != 0 {
		t.Errorf("got %d contacts from empty input, want 0", len(contacts))
	}
}

func TestParseVCard_MalformedInput(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN:No End Tag`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	// Should handle gracefully - no contacts parsed
	if len(contacts) != 0 {
		t.Errorf("got %d contacts from malformed input, want 0", len(contacts))
	}
}

func TestParseVCard_Address(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN:Jane Doe
ADR;TYPE=HOME:;;123 Main St;Springfield;IL;62701;USA
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}

	c := contacts[0]
	if len(c.Addresses) != 1 {
		t.Fatalf("got %d addresses, want 1", len(c.Addresses))
	}

	addr := c.Addresses[0]
	if addr.Street != "123 Main St" {
		t.Errorf("Street = %q, want %q", addr.Street, "123 Main St")
	}

	if addr.City != "Springfield" {
		t.Errorf("City = %q, want %q", addr.City, "Springfield")
	}

	if addr.Region != "IL" {
		t.Errorf("Region = %q, want %q", addr.Region, "IL")
	}

	if addr.PostalCode != "62701" {
		t.Errorf("PostalCode = %q, want %q", addr.PostalCode, "62701")
	}

	if addr.Country != "USA" {
		t.Errorf("Country = %q, want %q", addr.Country, "USA")
	}
}

func TestParseVCard_Birthday(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN:Test User
BDAY:1990-05-15
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}

	c := contacts[0]
	if c.Birthday == nil {
		t.Fatal("Birthday is nil")
	}

	expected := time.Date(1990, 5, 15, 0, 0, 0, 0, time.UTC)
	if !c.Birthday.Equal(expected) {
		t.Errorf("Birthday = %v, want %v", c.Birthday, expected)
	}
}

func TestParseVCard_MultipleEmails(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN:Multi Email
EMAIL;TYPE=HOME:home@example.com
EMAIL;TYPE=WORK:work@example.com
EMAIL;TYPE=PREF:primary@example.com
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}

	c := contacts[0]
	if len(c.Emails) != 3 {
		t.Fatalf("got %d emails, want 3", len(c.Emails))
	}

	// Check primary flag
	primaryCount := 0
	for _, email := range c.Emails {
		if email.Primary {
			primaryCount++
		}
	}

	if primaryCount != 1 {
		t.Errorf("got %d primary emails, want 1", primaryCount)
	}
}

func TestParseVCard_Groups(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
FN:Group Member
CATEGORIES:Friends,Colleagues,Family
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}

	c := contacts[0]
	if len(c.Groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(c.Groups))
	}

	expectedGroups := []string{"Friends", "Colleagues", "Family"}
	for i, expected := range expectedGroups {
		if c.Groups[i] != expected {
			t.Errorf("Group[%d] = %q, want %q", i, c.Groups[i], expected)
		}
	}
}

func TestParseVCard_UID(t *testing.T) {
	vcf := `BEGIN:VCARD
VERSION:3.0
UID:12345-abcde-67890-fghij
FN:UID Test
END:VCARD`

	contacts, err := ParseVCard(strings.NewReader(vcf))
	if err != nil {
		t.Fatalf("ParseVCard error: %v", err)
	}

	if len(contacts) != 1 {
		t.Fatalf("got %d contacts, want 1", len(contacts))
	}

	c := contacts[0]
	if c.UID != "12345-abcde-67890-fghij" {
		t.Errorf("UID = %q, want %q", c.UID, "12345-abcde-67890-fghij")
	}
}

func TestCSVHeaders(t *testing.T) {
	headers := CSVHeaders()
	if len(headers) == 0 {
		t.Error("CSVHeaders returned empty slice")
	}

	expectedHeaders := []string{"UID", "Full Name", "Given Name", "Family Name", "Email", "Phone", "Organization", "Title", "Birthday", "Notes"}
	if len(headers) != len(expectedHeaders) {
		t.Errorf("got %d headers, want %d", len(headers), len(expectedHeaders))
	}

	for i, expected := range expectedHeaders {
		if i >= len(headers) {
			break
		}
		if headers[i] != expected {
			t.Errorf("Header[%d] = %q, want %q", i, headers[i], expected)
		}
	}
}

func TestContactToCSVRow(t *testing.T) {
	birthday := time.Date(1985, 3, 20, 0, 0, 0, 0, time.UTC)
	contact := Contact{
		UID:          "test-uid",
		FullName:     "Test User",
		GivenName:    "Test",
		FamilyName:   "User",
		Emails:       []EmailEntry{{Address: "test@example.com"}},
		Phones:       []PhoneEntry{{Number: "555-1234"}},
		Organization: "Test Org",
		Title:        "Tester",
		Birthday:     &birthday,
		Notes:        "Test notes",
	}

	row := contact.ToCSVRow()
	if len(row) != 10 {
		t.Errorf("got %d CSV columns, want 10", len(row))
	}

	if row[0] != "test-uid" {
		t.Errorf("CSV[0] (UID) = %q, want %q", row[0], "test-uid")
	}

	if row[1] != "Test User" {
		t.Errorf("CSV[1] (Full Name) = %q, want %q", row[1], "Test User")
	}

	if row[4] != "test@example.com" {
		t.Errorf("CSV[4] (Email) = %q, want %q", row[4], "test@example.com")
	}
}
