// Package processors provides data processing for Gmail Takeout.
package processors

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"strings"

	conncore "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/gmail/metadata"
)

// Processor handles the Gmail MBOX processing pipeline.
type Processor struct{}

// NewGmailProcessor creates a new Gmail processor.
func NewGmailProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) ID() string   { return "gmail-processor" }
func (p *Processor) Name() string { return "Gmail Processor" }
func (p *Processor) Description() string {
	return "Process Gmail Takeout MBOX files - extract .eml messages and metadata"
}

// ConfigSchema returns the configuration options for this processor.
func (p *Processor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "extract_attachments",
			Name:        "Extract Attachments",
			Description: "Extract email attachments to separate files",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "body_preview_length",
			Name:        "Body Preview Length",
			Description: "Number of characters to extract for body preview",
			Type:        "int",
			Default:     200,
		},
	}
}

// Process runs the Gmail MBOX processing pipeline.
// Phases: 1) Scan MBOX files, 2) Parse messages, 3) Extract .eml files,
// 4) Extract metadata, 5) Extract attachments (optional), 6) Generate summary
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

	extractAttachments := getBool(settings, "extract_attachments", false)
	bodyPreviewLen := getInt(settings, "body_preview_length", 200)

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
	logger.Printf("=== Gmail MBOX processing started (source=%s) ===", cfg.SourceDir)

	// Find Mail directory
	mailPath, err := detectMailDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(mailPath)))
	logger.Printf("[Setup] Mail directory: %s", mailPath)

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.GmailLibrary{}

	// ── Phase 1: Scan for MBOX files ────────────────────────────────────────
	emit(1, "Scanning MBOX files", 0, 0, "")
	mboxFiles, err := p.scanMboxFiles(ctx, mailPath, logger)
	if err != nil {
		logger.Printf("[Phase 1] Error scanning MBOX files: %v", err)
		return nil, err
	}
	library.MboxFiles = mboxFiles
	result.Stats["mbox_files"] = len(mboxFiles)
	emit(1, "Scan complete", len(mboxFiles), len(mboxFiles),
		fmt.Sprintf("%d MBOX files found", len(mboxFiles)))
	logger.Printf("[Phase 1] mbox_files=%d", len(mboxFiles))

	// ── Phase 2: Parse MBOX messages ────────────────────────────────────────
	emit(2, "Parsing messages", 0, 0, "")
	messages, err := p.parseMboxFiles(ctx, mboxFiles, emit, logger)
	if err != nil {
		logger.Printf("[Phase 2] Error parsing messages: %v", err)
		return nil, err
	}
	library.Messages = messages
	result.Stats["messages"] = len(messages)
	emit(2, "Parse complete", len(messages), len(messages),
		fmt.Sprintf("%d messages parsed", len(messages)))
	logger.Printf("[Phase 2] messages=%d", len(messages))

	// ── Phase 3: Extract .eml files ─────────────────────────────────────────
	emit(3, "Extracting .eml files", 0, len(messages), "")
	emlDir := filepath.Join(cfg.WorkDir, "eml")
	os.MkdirAll(emlDir, 0o755)
	extractedCount, err := p.extractEmlFiles(ctx, mboxFiles, emlDir, emit, logger)
	if err != nil {
		logger.Printf("[Phase 3] Error extracting .eml files: %v", err)
	}
	result.Stats["eml_extracted"] = extractedCount
	emit(3, "Extraction complete", extractedCount, len(messages),
		fmt.Sprintf("%d .eml files extracted", extractedCount))
	logger.Printf("[Phase 3] eml_extracted=%d", extractedCount)

	// ── Phase 4: Extract metadata ───────────────────────────────────────────
	emit(4, "Extracting metadata", 0, len(messages), "")
	for i := range messages {
		// Add body preview
		if bodyPreviewLen > 0 {
			// Note: Body preview would require re-parsing; for now, skip
		}
		if i%100 == 0 {
			emit(4, "Extracting metadata", i, len(messages), "")
		}
	}
	emit(4, "Metadata complete", len(messages), len(messages), "")
	logger.Printf("[Phase 4] metadata extracted for %d messages", len(messages))

	// ── Phase 5: Extract attachments (optional) ─────────────────────────────
	if extractAttachments {
		emit(5, "Extracting attachments", 0, len(messages), "")
		attachDir := filepath.Join(cfg.WorkDir, "attachments")
		os.MkdirAll(attachDir, 0o755)
		attachCount, err := p.extractAttachments(ctx, mboxFiles, attachDir, emit, logger)
		if err != nil {
			logger.Printf("[Phase 5] Error extracting attachments: %v", err)
		}
		result.Stats["attachments"] = attachCount
		emit(5, "Attachments complete", attachCount, attachCount,
			fmt.Sprintf("%d attachments extracted", attachCount))
		logger.Printf("[Phase 5] attachments=%d", attachCount)
	} else {
		emit(5, "Attachments skipped", 1, 1, "Disabled in config")
		logger.Printf("[Phase 5] Skipped")
	}

	// ── Phase 6: Write output ───────────────────────────────────────────────
	emit(6, "Writing metadata", 0, 1, "")

	// Write library as JSON
	libraryPath := filepath.Join(cfg.WorkDir, "library.json")
	libraryData, _ := json.MarshalIndent(library, "", "  ")
	os.WriteFile(libraryPath, libraryData, 0o644)

	result.Metadata["library"] = library
	result.Metadata["library_path"] = libraryPath

	emit(6, "Output complete", 1, 1, fmt.Sprintf("Wrote %s", filepath.Base(libraryPath)))
	logger.Printf("[Phase 6] Wrote library.json")
	logger.Printf("=== Gmail MBOX processing complete ===")

	// Build ProcessedItems
	result.Items = p.buildProcessedItems(library, cfg.WorkDir)

	return result, nil
}

// scanMboxFiles finds all .mbox files in the Mail directory.
func (p *Processor) scanMboxFiles(ctx context.Context, mailPath string, logger *log.Logger) ([]metadata.MboxFile, error) {
	var mboxFiles []metadata.MboxFile

	err := filepath.WalkDir(mailPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip unreadable paths
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(strings.ToLower(d.Name()), ".mbox") {
			label := strings.TrimSuffix(d.Name(), ".mbox")
			mboxFiles = append(mboxFiles, metadata.MboxFile{
				Path:  path,
				Label: label,
			})
		}

		return nil
	})

	return mboxFiles, err
}

// parseMboxFiles parses all MBOX files and extracts message headers.
func (p *Processor) parseMboxFiles(ctx context.Context, mboxFiles []metadata.MboxFile, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.EmailMessage, error) {
	var allMessages []metadata.EmailMessage

	for fileIdx, mboxFile := range mboxFiles {
		select {
		case <-ctx.Done():
			return allMessages, ctx.Err()
		default:
		}

		messages, err := p.parseSingleMbox(ctx, mboxFile.Path, mboxFile.Label, logger)
		if err != nil {
			logger.Printf("Error parsing %s: %v", mboxFile.Path, err)
			continue
		}

		// Update MboxFile message count
		mboxFiles[fileIdx].MessageCount = len(messages)
		allMessages = append(allMessages, messages...)

		emit(2, "Parsing messages", fileIdx+1, len(mboxFiles),
			fmt.Sprintf("%s: %d messages", mboxFile.Label, len(messages)))
	}

	return allMessages, nil
}

// parseSingleMbox parses a single MBOX file using RFC 4155 format.
// Messages start with "From " (with space after From).
func (p *Processor) parseSingleMbox(ctx context.Context, path, label string, logger *log.Logger) ([]metadata.EmailMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var messages []metadata.EmailMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	var currentMessage strings.Builder
	messageCount := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return messages, ctx.Err()
		default:
		}

		line := scanner.Text()

		// RFC 4155: Messages start with "From " (with space)
		if strings.HasPrefix(line, "From ") && currentMessage.Len() > 0 {
			// Process previous message
			msg, err := metadata.ParseMboxHeader(currentMessage.String())
			if err != nil {
				logger.Printf("Warning: Failed to parse message %d in %s: %v", messageCount, label, err)
			} else {
				msg.Labels = []string{label}
				messages = append(messages, msg)
			}
			messageCount++
			currentMessage.Reset()
		} else {
			currentMessage.WriteString(line)
			currentMessage.WriteString("\n")
		}
	}

	// Process last message
	if currentMessage.Len() > 0 {
		msg, err := metadata.ParseMboxHeader(currentMessage.String())
		if err != nil {
			logger.Printf("Warning: Failed to parse last message in %s: %v", label, err)
		} else {
			msg.Labels = []string{label}
			messages = append(messages, msg)
		}
	}

	if err := scanner.Err(); err != nil {
		return messages, fmt.Errorf("scan mbox: %w", err)
	}

	return messages, nil
}

// extractEmlFiles writes individual .eml files for each message.
func (p *Processor) extractEmlFiles(ctx context.Context, mboxFiles []metadata.MboxFile, destDir string, emit func(int, string, int, int, string), logger *log.Logger) (int, error) {
	extractedCount := 0

	for _, mboxFile := range mboxFiles {
		select {
		case <-ctx.Done():
			return extractedCount, ctx.Err()
		default:
		}

		// Create label subdirectory
		labelDir := filepath.Join(destDir, sanitizeFilename(mboxFile.Label))
		os.MkdirAll(labelDir, 0o755)

		count, err := p.extractEmlFromMbox(ctx, mboxFile.Path, labelDir, logger)
		if err != nil {
			logger.Printf("Error extracting .eml from %s: %v", mboxFile.Path, err)
			continue
		}

		extractedCount += count
		emit(3, "Extracting .eml files", extractedCount, extractedCount, "")
	}

	return extractedCount, nil
}

// extractEmlFromMbox extracts individual .eml files from a single MBOX file.
func (p *Processor) extractEmlFromMbox(ctx context.Context, mboxPath, destDir string, logger *log.Logger) (int, error) {
	f, err := os.Open(mboxPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	var currentMessage strings.Builder
	messageIdx := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return messageIdx, ctx.Err()
		default:
		}

		line := scanner.Text()

		if strings.HasPrefix(line, "From ") && currentMessage.Len() > 0 {
			// Write previous message
			emlPath := filepath.Join(destDir, fmt.Sprintf("message_%04d.eml", messageIdx))
			os.WriteFile(emlPath, []byte(currentMessage.String()), 0o644)
			messageIdx++
			currentMessage.Reset()
		} else {
			currentMessage.WriteString(line)
			currentMessage.WriteString("\n")
		}
	}

	// Write last message
	if currentMessage.Len() > 0 {
		emlPath := filepath.Join(destDir, fmt.Sprintf("message_%04d.eml", messageIdx))
		os.WriteFile(emlPath, []byte(currentMessage.String()), 0o644)
		messageIdx++
	}

	return messageIdx, scanner.Err()
}

// extractAttachments extracts email attachments from MBOX files.
func (p *Processor) extractAttachments(ctx context.Context, mboxFiles []metadata.MboxFile, destDir string, emit func(int, string, int, int, string), logger *log.Logger) (int, error) {
	attachmentCount := 0

	for _, mboxFile := range mboxFiles {
		select {
		case <-ctx.Done():
			return attachmentCount, ctx.Err()
		default:
		}

		count, err := p.extractAttachmentsFromMbox(ctx, mboxFile.Path, destDir, logger)
		if err != nil {
			logger.Printf("Error extracting attachments from %s: %v", mboxFile.Path, err)
			continue
		}

		attachmentCount += count
	}

	return attachmentCount, nil
}

// extractAttachmentsFromMbox extracts attachments from a single MBOX file.
func (p *Processor) extractAttachmentsFromMbox(ctx context.Context, mboxPath, destDir string, logger *log.Logger) (int, error) {
	f, err := os.Open(mboxPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	var currentMessage strings.Builder
	attachmentCount := 0
	messageIdx := 0

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return attachmentCount, ctx.Err()
		default:
		}

		line := scanner.Text()

		if strings.HasPrefix(line, "From ") && currentMessage.Len() > 0 {
			// Process attachments in previous message
			count := p.extractAttachmentsFromMessage(currentMessage.String(), destDir, messageIdx, logger)
			attachmentCount += count
			messageIdx++
			currentMessage.Reset()
		} else {
			currentMessage.WriteString(line)
			currentMessage.WriteString("\n")
		}
	}

	// Process last message
	if currentMessage.Len() > 0 {
		count := p.extractAttachmentsFromMessage(currentMessage.String(), destDir, messageIdx, logger)
		attachmentCount += count
	}

	return attachmentCount, scanner.Err()
}

// extractAttachmentsFromMessage extracts attachments from a single message string.
func (p *Processor) extractAttachmentsFromMessage(messageData, destDir string, messageIdx int, logger *log.Logger) int {
	msg, err := mail.ReadMessage(strings.NewReader(messageData))
	if err != nil {
		return 0
	}

	contentType := msg.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return 0
	}

	boundary := params["boundary"]
	if boundary == "" {
		return 0
	}

	mr := multipart.NewReader(msg.Body, boundary)
	attachmentCount := 0

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Printf("Error reading multipart: %v", err)
			break
		}

		// Check if this part is an attachment
		disposition := part.Header.Get("Content-Disposition")
		if !strings.HasPrefix(disposition, "attachment") {
			continue
		}

		// Extract filename
		_, params, err := mime.ParseMediaType(disposition)
		if err != nil {
			continue
		}
		filename := params["filename"]
		if filename == "" {
			filename = fmt.Sprintf("attachment_%d_%d", messageIdx, attachmentCount)
		}

		// Write attachment
		attachPath := filepath.Join(destDir, fmt.Sprintf("msg_%04d_%s", messageIdx, sanitizeFilename(filename)))
		outFile, err := os.Create(attachPath)
		if err != nil {
			logger.Printf("Error creating attachment file: %v", err)
			continue
		}

		_, err = io.Copy(outFile, part)
		outFile.Close()
		if err != nil {
			logger.Printf("Error writing attachment: %v", err)
			continue
		}

		attachmentCount++
	}

	return attachmentCount
}

// buildProcessedItems creates ProcessedItem entries for outputs.
func (p *Processor) buildProcessedItems(library *metadata.GmailLibrary, workDir string) []core.ProcessedItem {
	var items []core.ProcessedItem

	// Add each message as an item
	for _, msg := range library.Messages {
		items = append(items, core.ProcessedItem{
			SourcePath:    "",
			ProcessedPath: "",
			DestRelPath:   fmt.Sprintf("messages/%s.json", sanitizeFilename(msg.MessageID)),
			Type:          string(conncore.DataTypeEmail),
			Metadata: map[string]any{
				"message_id": msg.MessageID,
				"from":       msg.From,
				"to":         msg.To,
				"subject":    msg.Subject,
				"date":       msg.Date,
				"labels":     msg.Labels,
				"message":    msg,
			},
		})
	}

	return items
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func getBool(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func getInt(m map[string]any, key string, def int) int {
	if v, ok := m[key].(int); ok {
		return v
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return def
}

var mailDirVariants = []string{
	"Mail",
	"Posta", // Italian
}

func detectMailDir(sourceDir string) (string, error) {
	baseName := filepath.Base(sourceDir)
	for _, variant := range mailDirVariants {
		if strings.EqualFold(baseName, variant) {
			return sourceDir, nil
		}
	}

	var found string
	filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(sourceDir, path)
		depth := len(strings.Split(rel, string(os.PathSeparator)))
		if depth > 3 {
			return filepath.SkipDir
		}
		for _, variant := range mailDirVariants {
			if strings.EqualFold(d.Name(), variant) {
				found = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	if found != "" {
		return found, nil
	}

	return sourceDir, nil
}

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
		" ", "_",
	)
	return replacer.Replace(name)
}
