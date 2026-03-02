// Package processors provides data processing for Google Contacts Takeout.
package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	conncore "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/services/contacts/metadata"
	"github.com/fulgidus/revoco/services/core"
)

// Processor handles the Google Contacts vCard processing pipeline.
type Processor struct{}

// NewContactsProcessor creates a new Contacts processor.
func NewContactsProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "contacts-processor" }
func (p *Processor) Name() string { return "Contacts Processor" }
func (p *Processor) Description() string {
	return "Process Google Contacts Takeout vCard (.vcf) files - extract contact metadata"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "merge_duplicates",
			Name:        "Merge Duplicates",
			Description: "Merge duplicate contacts with same email",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "extract_photos",
			Name:        "Extract Photos",
			Description: "Extract contact photos to separate files",
			Type:        "bool",
			Default:     false,
		},
	}
}

// Process runs the Google Contacts vCard processing pipeline.
// Phases: 1) Scan .vcf files, 2) Parse vCards, 3) Extract metadata,
// 4) Normalize data, 5) Generate summary
func (p *Processor) Process(ctx context.Context, cfg core.ProcessorConfig, events chan<- core.ProgressEvent) (*core.ProcessResult, error) {
	defer close(events)

	emit := func(phase int, label string, done, total int, msg string) {
		select {
		case events <- core.ProgressEvent{
			Phase:   phase,
			Label:   label,
			Done:    done,
			Total:   total,
			Message: msg,
		}:
		case <-ctx.Done():
		}
	}

	settings := cfg.Settings
	if settings == nil {
		settings = make(map[string]any)
	}

	mergeDuplicates := getBool(settings, "merge_duplicates", false)
	extractPhotos := getBool(settings, "extract_photos", false)

	// Setup logging
	logDir := cfg.SessionDir
	if logDir == "" {
		logDir = cfg.WorkDir
	}
	os.MkdirAll(logDir, 0o755)

	logPath := filepath.Join(logDir, "process.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("=== Contacts vCard processing started (source=%s) ===", cfg.SourceDir)

	// Find Contacts directory
	contactsPath, err := detectContactsDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(contactsPath)))
	logger.Printf("[Setup] Contacts directory: %s", contactsPath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.ContactsLibrary{}

	// ── Phase 1: Scan for .vcf files ─────────────────────────────────────
	emit(1, "Scanning .vcf files", 0, 0, "")
	vcfFiles, err := p.scanVcfFiles(ctx, contactsPath, logger)
	if err != nil {
		logger.Printf("[Phase 1] Error scanning .vcf files: %v", err)
		return nil, err
	}
	result.Stats["vcf_files"] = len(vcfFiles)
	emit(1, "Scan complete", len(vcfFiles), len(vcfFiles),
		fmt.Sprintf("%d .vcf files found", len(vcfFiles)))
	logger.Printf("[Phase 1] vcf_files=%d", len(vcfFiles))

	// ── Phase 2: Parse vCard files ───────────────────────────────────────
	emit(2, "Parsing vCards", 0, 0, "")
	contacts, err := p.parseVcfFiles(ctx, vcfFiles, emit, logger)
	if err != nil {
		logger.Printf("[Phase 2] Error parsing vCards: %v", err)
		return nil, err
	}
	library.Contacts = contacts
	result.Stats["contacts"] = len(contacts)
	emit(2, "Parse complete", len(contacts), len(contacts),
		fmt.Sprintf("%d contacts parsed", len(contacts)))
	logger.Printf("[Phase 2] contacts=%d", len(contacts))

	// ── Phase 3: Extract metadata ─────────────────────────────────────────
	emit(3, "Extracting metadata", 0, len(contacts), "")
	processedItems, err := p.extractMetadata(ctx, contacts, cfg.WorkDir, extractPhotos, emit, logger)
	if err != nil {
		logger.Printf("[Phase 3] Error extracting metadata: %v", err)
	}
	result.Items = processedItems
	result.Stats["processed_contacts"] = len(processedItems)
	emit(3, "Metadata extraction complete", len(processedItems), len(contacts),
		fmt.Sprintf("%d contacts processed", len(processedItems)))
	logger.Printf("[Phase 3] processed_contacts=%d", len(processedItems))

	// ── Phase 4: Normalize data ────────────────────────────────────────────
	emit(4, "Normalizing data", 0, len(contacts), "")
	stats := p.normalizeData(ctx, contacts, mergeDuplicates, emit, logger)
	for k, v := range stats {
		result.Stats[k] = v
	}
	emit(4, "Normalization complete", len(contacts), len(contacts),
		fmt.Sprintf("%d total contacts", result.Stats["normalized_contacts"]))
	logger.Printf("[Phase 4] normalization stats=%+v", stats)

	// ── Phase 5: Generate summary ──────────────────────────────────────────
	emit(5, "Generating summary", 0, 1, "")
	summary, err := p.generateSummary(ctx, library, result.Stats)
	if err != nil {
		logger.Printf("[Phase 5] Error generating summary: %v", err)
	} else {
		result.Metadata = summary
		summaryPath := filepath.Join(cfg.WorkDir, "contacts-summary.json")
		if jsonData, err := json.MarshalIndent(summary, "", "  "); err == nil {
			os.WriteFile(summaryPath, jsonData, 0o644)
			logger.Printf("[Phase 5] Summary written to: %s", summaryPath)
		}
	}
	emit(5, "Processing complete", 1, 1, "All contacts processed")

	logger.Printf("=== Contacts processing complete: %d contacts, %d files ===",
		result.Stats["contacts"], result.Stats["vcf_files"])

	return result, nil
}

// detectContactsDir finds the Contacts directory in the source path.
func detectContactsDir(sourcePath string) (string, error) {
	variants := []string{
		"Contacts",
		"Contatti",  // Italian
		"Contactos", // Spanish
		"Kontakte",  // German
	}

	for _, variant := range variants {
		path := filepath.Join(sourcePath, variant)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}

	// Fallback: use source path if it contains .vcf files
	if files, err := os.ReadDir(sourcePath); err == nil {
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".vcf") {
				return sourcePath, nil
			}
		}
	}

	return "", fmt.Errorf("contacts directory not found (tried: %v)", variants)
}

// scanVcfFiles finds all .vcf files in the contacts directory.
func (p *Processor) scanVcfFiles(ctx context.Context, contactsPath string, logger *log.Logger) ([]string, error) {
	var vcfFiles []string

	err := filepath.Walk(contactsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !info.IsDir() && (strings.HasSuffix(strings.ToLower(info.Name()), ".vcf") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".vcard")) {
			vcfFiles = append(vcfFiles, path)
			logger.Printf("[Scan] Found: %s", filepath.Base(path))
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk contacts dir: %w", err)
	}

	return vcfFiles, nil
}

// parseVcfFiles parses all vCard files and returns contacts.
func (p *Processor) parseVcfFiles(ctx context.Context, vcfFiles []string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.Contact, error) {
	var allContacts []metadata.Contact
	total := len(vcfFiles)

	for i, vcfFile := range vcfFiles {
		// Check cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emit(2, "Parsing vCards", i, total, filepath.Base(vcfFile))

		file, err := os.Open(vcfFile)
		if err != nil {
			logger.Printf("[Parse] Error opening %s: %v", vcfFile, err)
			continue
		}

		contacts, err := metadata.ParseVCard(file)
		file.Close()

		if err != nil {
			logger.Printf("[Parse] Error parsing %s: %v", vcfFile, err)
			continue
		}

		allContacts = append(allContacts, contacts...)
		logger.Printf("[Parse] %s: %d contacts", filepath.Base(vcfFile), len(contacts))
	}

	return allContacts, nil
}

// extractMetadata extracts metadata from contacts and creates ProcessedItems.
func (p *Processor) extractMetadata(ctx context.Context, contacts []metadata.Contact, workDir string, extractPhotos bool, emit func(int, string, int, int, string), logger *log.Logger) ([]core.ProcessedItem, error) {
	items := make([]core.ProcessedItem, 0, len(contacts))
	total := len(contacts)

	for i, contact := range contacts {
		// Check cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emit(3, "Extracting metadata", i, total, contact.FullName)

		// Create metadata map
		contactMeta := map[string]any{
			"uid":          contact.UID,
			"full_name":    contact.FullName,
			"given_name":   contact.GivenName,
			"family_name":  contact.FamilyName,
			"organization": contact.Organization,
			"title":        contact.Title,
			"version":      contact.Version,
		}

		if len(contact.Emails) > 0 {
			contactMeta["emails"] = contact.Emails
		}
		if len(contact.Phones) > 0 {
			contactMeta["phones"] = contact.Phones
		}
		if len(contact.Addresses) > 0 {
			contactMeta["addresses"] = contact.Addresses
		}
		if contact.Birthday != nil {
			contactMeta["birthday"] = contact.Birthday.Format("2006-01-02")
		}
		if contact.Notes != "" {
			contactMeta["notes"] = contact.Notes
		}

		// Extract photo if enabled
		if extractPhotos && contact.Photo != "" {
			photoPath, err := p.extractPhoto(contact, workDir, logger)
			if err != nil {
				logger.Printf("[Extract] Error extracting photo for %s: %v", contact.FullName, err)
			} else if photoPath != "" {
				contactMeta["photo_path"] = photoPath
			}
		}

		// Generate destination path
		destRel := generateContactPath(contact)

		item := core.ProcessedItem{
			SourcePath:    "", // No specific source file per contact
			ProcessedPath: "",
			DestRelPath:   destRel,
			Type:          string(conncore.DataTypeContact),
			Metadata:      contactMeta,
		}

		items = append(items, item)
	}

	return items, nil
}

// extractPhoto extracts a contact photo to a file.
func (p *Processor) extractPhoto(contact metadata.Contact, workDir string, logger *log.Logger) (string, error) {
	// Photo extraction implementation (base64 decode)
	// TODO: Implement when needed
	return "", nil
}

// normalizeData normalizes contact data (e.g., merge duplicates).
func (p *Processor) normalizeData(ctx context.Context, contacts []metadata.Contact, mergeDuplicates bool, emit func(int, string, int, int, string), logger *log.Logger) map[string]int {
	stats := make(map[string]int)
	stats["normalized_contacts"] = len(contacts)

	// Count contacts with various fields
	emailCount := 0
	phoneCount := 0
	addressCount := 0
	photoCount := 0
	orgCount := 0

	for i, contact := range contacts {
		select {
		case <-ctx.Done():
			return stats
		default:
		}

		emit(4, "Normalizing data", i, len(contacts), contact.FullName)

		if len(contact.Emails) > 0 {
			emailCount++
		}
		if len(contact.Phones) > 0 {
			phoneCount++
		}
		if len(contact.Addresses) > 0 {
			addressCount++
		}
		if contact.Photo != "" {
			photoCount++
		}
		if contact.Organization != "" {
			orgCount++
		}
	}

	stats["contacts_with_email"] = emailCount
	stats["contacts_with_phone"] = phoneCount
	stats["contacts_with_address"] = addressCount
	stats["contacts_with_photo"] = photoCount
	stats["contacts_with_organization"] = orgCount

	// TODO: Implement duplicate merging if mergeDuplicates is true

	return stats
}

// generateSummary creates a summary of the processed contacts.
func (p *Processor) generateSummary(ctx context.Context, library *metadata.ContactsLibrary, stats map[string]int) (map[string]any, error) {
	summary := map[string]any{
		"total_contacts":             library != nil && len(library.Contacts) > 0,
		"statistics":                 stats,
		"contacts_with_email":        stats["contacts_with_email"],
		"contacts_with_phone":        stats["contacts_with_phone"],
		"contacts_with_address":      stats["contacts_with_address"],
		"contacts_with_photo":        stats["contacts_with_photo"],
		"contacts_with_organization": stats["contacts_with_organization"],
	}

	if library != nil {
		summary["total_contacts"] = len(library.Contacts)

		// Sample contacts (first 5)
		sampleSize := 5
		if len(library.Contacts) < sampleSize {
			sampleSize = len(library.Contacts)
		}
		if sampleSize > 0 {
			samples := make([]map[string]any, sampleSize)
			for i := 0; i < sampleSize; i++ {
				c := library.Contacts[i]
				samples[i] = map[string]any{
					"full_name":    c.FullName,
					"organization": c.Organization,
					"email_count":  len(c.Emails),
					"phone_count":  len(c.Phones),
				}
			}
			summary["sample_contacts"] = samples
		}
	}

	return summary, nil
}

// generateContactPath generates a destination path for a contact.
func generateContactPath(contact metadata.Contact) string {
	// Use UID if available, otherwise generate from name
	if contact.UID != "" {
		return fmt.Sprintf("contacts/%s.vcf", sanitizeFilename(contact.UID))
	}
	if contact.FullName != "" {
		return fmt.Sprintf("contacts/%s.vcf", sanitizeFilename(contact.FullName))
	}
	// Fallback to email
	if len(contact.Emails) > 0 {
		return fmt.Sprintf("contacts/%s.vcf", sanitizeFilename(contact.Emails[0].Address))
	}
	return "contacts/unknown.vcf"
}

// sanitizeFilename removes invalid characters from filename.
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}

// getBool extracts a bool from settings map.
func getBool(settings map[string]any, key string, defaultVal bool) bool {
	if v, ok := settings[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// getInt extracts an int from settings map.
func getInt(settings map[string]any, key string, defaultVal int) int {
	if v, ok := settings[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return defaultVal
}
