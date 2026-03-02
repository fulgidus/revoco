// Package outputs provides output modules for Google Contacts data.
package outputs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/contacts/metadata"
	"github.com/fulgidus/revoco/services/core"
)

// ── VCF Output ───────────────────────────────────────────────────────────────

// VCFOutput exports contacts as vCard 3.0 files.
type VCFOutput struct {
	destDir      string
	singleFile   bool
	outputFormat string // "3.0" or "4.0"
}

// NewVCF creates a new VCF output.
func NewVCF() *VCFOutput {
	return &VCFOutput{
		singleFile:   false,
		outputFormat: "3.0",
	}
}

func (o *VCFOutput) ID() string          { return "contacts-vcf" }
func (o *VCFOutput) Name() string        { return "Contacts VCF Export" }
func (o *VCFOutput) Description() string { return "Export contacts as vCard (.vcf) files" }

func (o *VCFOutput) SupportedItemTypes() []string {
	return []string{"contact"}
}

func (o *VCFOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for vCard files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "single_file",
			Name:        "Single File",
			Description: "Export all contacts to a single .vcf file",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "format",
			Name:        "vCard Format",
			Description: "vCard version (3.0 or 4.0)",
			Type:        "select",
			Options:     []string{"3.0", "4.0"},
			Default:     "3.0",
		},
	}
}

func (o *VCFOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["single_file"].(bool); ok {
		o.singleFile = v
	}

	if v, ok := cfg.Settings["format"].(string); ok {
		o.outputFormat = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *VCFOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "contact" {
		return nil
	}

	// Extract contact from metadata
	contact, err := extractContact(item.Metadata)
	if err != nil {
		return fmt.Errorf("extract contact: %w", err)
	}

	// Generate vCard content
	vcardContent := o.generateVCard(contact)

	// Write to file
	destPath := filepath.Join(o.destDir, item.DestRelPath)
	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Ensure .vcf extension
	if !strings.HasSuffix(destPath, ".vcf") {
		destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".vcf"
	}

	return os.WriteFile(destPath, []byte(vcardContent), 0o644)
}

func (o *VCFOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	if o.singleFile {
		return o.exportSingleFile(ctx, items, progress)
	}

	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *VCFOutput) exportSingleFile(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	outputPath := filepath.Join(o.destDir, "contacts.vcf")
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	for i, item := range items {
		if item.Type != "contact" {
			continue
		}

		contact, err := extractContact(item.Metadata)
		if err != nil {
			continue
		}

		vcardContent := o.generateVCard(contact)
		if _, err := file.WriteString(vcardContent + "\n"); err != nil {
			return fmt.Errorf("write vcard: %w", err)
		}

		if progress != nil {
			progress(i+1, len(items))
		}
	}

	return nil
}

func (o *VCFOutput) generateVCard(contact metadata.Contact) string {
	var sb strings.Builder

	sb.WriteString("BEGIN:VCARD\n")
	sb.WriteString(fmt.Sprintf("VERSION:%s\n", o.outputFormat))

	// Full name
	if contact.FullName != "" {
		sb.WriteString(fmt.Sprintf("FN:%s\n", escapeVCard(contact.FullName)))
	}

	// Structured name (N)
	n := fmt.Sprintf("N:%s;%s;%s;%s;%s",
		escapeVCard(contact.FamilyName),
		escapeVCard(contact.GivenName),
		escapeVCard(contact.MiddleName),
		escapeVCard(contact.Prefix),
		escapeVCard(contact.Suffix))
	sb.WriteString(n + "\n")

	// Nickname
	if contact.Nickname != "" {
		sb.WriteString(fmt.Sprintf("NICKNAME:%s\n", escapeVCard(contact.Nickname)))
	}

	// Emails
	for _, email := range contact.Emails {
		typeStr := ""
		if len(email.Type) > 0 {
			typeStr = ";TYPE=" + strings.Join(email.Type, ",")
		}
		sb.WriteString(fmt.Sprintf("EMAIL%s:%s\n", typeStr, escapeVCard(email.Address)))
	}

	// Phones
	for _, phone := range contact.Phones {
		typeStr := ""
		if len(phone.Type) > 0 {
			typeStr = ";TYPE=" + strings.Join(phone.Type, ",")
		}
		sb.WriteString(fmt.Sprintf("TEL%s:%s\n", typeStr, escapeVCard(phone.Number)))
	}

	// Addresses
	for _, addr := range contact.Addresses {
		typeStr := ""
		if len(addr.Type) > 0 {
			typeStr = ";TYPE=" + strings.Join(addr.Type, ",")
		}
		adrStr := fmt.Sprintf("ADR%s:;;%s;%s;%s;%s;%s",
			typeStr,
			escapeVCard(addr.Street),
			escapeVCard(addr.City),
			escapeVCard(addr.Region),
			escapeVCard(addr.PostalCode),
			escapeVCard(addr.Country))
		sb.WriteString(adrStr + "\n")
	}

	// Organization
	if contact.Organization != "" {
		sb.WriteString(fmt.Sprintf("ORG:%s\n", escapeVCard(contact.Organization)))
	}

	// Title
	if contact.Title != "" {
		sb.WriteString(fmt.Sprintf("TITLE:%s\n", escapeVCard(contact.Title)))
	}

	// Birthday
	if contact.Birthday != nil {
		sb.WriteString(fmt.Sprintf("BDAY:%s\n", contact.Birthday.Format("2006-01-02")))
	}

	// Notes
	if contact.Notes != "" {
		sb.WriteString(fmt.Sprintf("NOTE:%s\n", escapeVCard(contact.Notes)))
	}

	// URL
	if contact.URL != "" {
		sb.WriteString(fmt.Sprintf("URL:%s\n", escapeVCard(contact.URL)))
	}

	// Photo (base64 encoded)
	if contact.Photo != "" {
		sb.WriteString("PHOTO;ENCODING=BASE64;TYPE=JPEG:")
		sb.WriteString(contact.Photo)
		sb.WriteString("\n")
	}

	// Categories/Groups
	if len(contact.Groups) > 0 {
		sb.WriteString(fmt.Sprintf("CATEGORIES:%s\n", strings.Join(contact.Groups, ",")))
	}

	// UID
	if contact.UID != "" {
		sb.WriteString(fmt.Sprintf("UID:%s\n", escapeVCard(contact.UID)))
	}

	sb.WriteString("END:VCARD")

	return sb.String()
}

func (o *VCFOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── JSON Output ──────────────────────────────────────────────────────────────

// JSONOutput exports contacts to JSON files.
type JSONOutput struct {
	destDir    string
	pretty     bool
	singleFile bool
}

// NewJSON creates a new JSON output.
func NewJSON() *JSONOutput {
	return &JSONOutput{
		pretty:     true,
		singleFile: false,
	}
}

func (o *JSONOutput) ID() string          { return "contacts-json" }
func (o *JSONOutput) Name() string        { return "Contacts JSON Export" }
func (o *JSONOutput) Description() string { return "Export contacts to JSON format" }

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"contact"}
}

func (o *JSONOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for JSON files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "pretty",
			Name:        "Pretty Print",
			Description: "Format JSON with indentation",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "single_file",
			Name:        "Single File",
			Description: "Export all contacts to a single JSON file",
			Type:        "bool",
			Default:     false,
		},
	}
}

func (o *JSONOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["pretty"].(bool); ok {
		o.pretty = v
	}

	if v, ok := cfg.Settings["single_file"].(bool); ok {
		o.singleFile = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *JSONOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "contact" {
		return nil
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Ensure .json extension
	if !strings.HasSuffix(destPath, ".json") {
		destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".json"
	}

	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(item.Metadata, "", "  ")
	} else {
		data, err = json.Marshal(item.Metadata)
	}
	if err != nil {
		return err
	}

	return os.WriteFile(destPath, data, 0o644)
}

func (o *JSONOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	if o.singleFile {
		return o.exportSingleFile(ctx, items, progress)
	}

	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *JSONOutput) exportSingleFile(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	outputPath := filepath.Join(o.destDir, "contacts.json")

	contacts := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item.Type == "contact" {
			contacts = append(contacts, item.Metadata)
		}
	}

	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(contacts, "", "  ")
	} else {
		data, err = json.Marshal(contacts)
	}
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, data, 0o644)
}

func (o *JSONOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── CSV Output ───────────────────────────────────────────────────────────────

// CSVOutput exports contacts to CSV format.
type CSVOutput struct {
	destDir string
}

// NewCSV creates a new CSV output.
func NewCSV() *CSVOutput {
	return &CSVOutput{}
}

func (o *CSVOutput) ID() string          { return "contacts-csv" }
func (o *CSVOutput) Name() string        { return "Contacts CSV Export" }
func (o *CSVOutput) Description() string { return "Export contacts to CSV format" }

func (o *CSVOutput) SupportedItemTypes() []string {
	return []string{"contact"}
}

func (o *CSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for CSV files",
			Type:        "string",
			Required:    true,
		},
	}
}

func (o *CSVOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *CSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// CSV export is batch-only
	return nil
}

func (o *CSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	outputPath := filepath.Join(o.destDir, "contacts.csv")
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create csv file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write(metadata.CSVHeaders()); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	// Write rows
	for i, item := range items {
		if item.Type != "contact" {
			continue
		}

		contact, err := extractContact(item.Metadata)
		if err != nil {
			continue
		}

		row := contact.ToCSVRow()
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}

		if progress != nil {
			progress(i+1, len(items))
		}
	}

	return nil
}

func (o *CSVOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Helper Functions ─────────────────────────────────────────────────────────

// extractContact extracts a Contact struct from metadata map.
func extractContact(meta map[string]any) (metadata.Contact, error) {
	// Convert metadata back to Contact struct
	jsonData, err := json.Marshal(meta)
	if err != nil {
		return metadata.Contact{}, fmt.Errorf("marshal metadata: %w", err)
	}

	var contact metadata.Contact
	if err := json.Unmarshal(jsonData, &contact); err != nil {
		return metadata.Contact{}, fmt.Errorf("unmarshal contact: %w", err)
	}

	return contact, nil
}

// escapeVCard escapes special characters in vCard values.
func escapeVCard(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, ";", "\\;")
	return s
}

// ── Registration ─────────────────────────────────────────────────────────────

func init() {
	core.RegisterOutput(NewVCF())
	core.RegisterOutput(NewJSON())
	core.RegisterOutput(NewCSV())
}
