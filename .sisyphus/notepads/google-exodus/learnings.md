# Learnings — Google Exodus

> Conventions, patterns, and gotchas discovered during implementation

---

## [INIT] Session: ses_353ba6eb1ffeGwDKcQ5pCzqD0Y

### Plan Structure
- 19 total tasks: 15 implementation + 4 final verification
- 5 waves + final verification wave
- Wave 1 (foundation): Tasks 1-4 (housekeeping, shared ingesters, DataTypes, YTMusic tests)
- Wave 2-4: New service processors in priority tiers
- Wave 5: Migrate existing services to shared ingesters
- Final: 4 parallel verification agents

### Critical Wiring Pattern (ALL new services)
Every new service MUST:
1. Add blank import to `services/register.go`: `_ "github.com/fulgidus/revoco/services/<id>"`
2. Import outputs in `service.go`: `_ "github.com/fulgidus/revoco/services/<id>/outputs"`
3. Test registration: `core.GetService("<id>")` returns non-nil

### Repository Context
- Git flow: feature branches off `develop`
- Worktree: `/home/fulgidus/Documents/revoco-google-exodus` on branch `feature/google-exodus`
- Main repo: `/home/fulgidus/Documents/revoco` on `develop`
- Package naming quirk: `connectors/` declares `package core` NOT `package connectors`

---

## [Task 4] YouTube Music Processor Tests

### Test Pattern Used
- **Test fixture creation**: Helper functions create synthetic Takeout directories with minimal CSV/file structure
- **t.TempDir()**: Standard pattern for isolated test directories
- **Progress event collection**: Buffered channel + drain pattern to validate progress reporting
- **Edge cases**: Empty data, malformed CSVs, context cancellation all handled gracefully

### Test Coverage Achieved: 86.8%
Tests written for:
- **Metadata**: ID(), Name(), Description(), ConfigSchema()
- **Full processing pipeline**: All phases (playlists, liked songs, uploads, subscriptions, local matching)
- **Selective phase processing**: Config flags to enable/disable phases
- **Empty Takeout**: Graceful handling of no data
- **Malformed data**: Broken CSVs don't crash processor
- **Cancellation**: Context cancellation mid-processing
- **Individual scanners**: scanPlaylists, scanLikedSongs, scanUploads, scanSubscriptions, matchLocalLibrary
- **Helper functions**: getBool, getString, isAudioExt, findLocalMatch, detectYouTubeMusicDir, buildProcessedItems

### Key Testing Insights
1. **Processor resilience**: Errors in individual phases are logged but don't stop pipeline
2. **Partial matching logic**: findLocalMatch tries exact → artist-title → substring (intentionally permissive)
3. **Directory detection**: detectYouTubeMusicDir supports multiple language variants (English, Italian)
4. **Audio extension detection**: 8 formats supported (.mp3, .flac, .m4a, .wav, .ogg, .opus, .aac, .wma)
5. **Progress tracking**: Emits events for all 6 phases (scan playlists, liked, uploads, subs, local match, output)

### Test Fixture Structure
```
YouTube and YouTube Music/
  playlists/
    My Playlist.csv          # VideoID, TimeAdded, Title
  liked music.csv            # VideoID, TimeAdded, Title
  subscriptions.csv          # ChannelID, ChannelURL, ChannelName
  uploads/
    My Song.mp3              # Fake audio file
    Another Track.flac       # Fake audio file
```

### Patterns to Reuse
- **Synthetic fixtures > real data**: Minimal CSV structure, no privacy concerns
- **Helper separation**: createTestTakeout(), createEmptyTakeout(), createMalformedTakeout()
- **Evidence generation**: Redirect test output to .sisyphus/evidence/*.txt
- **Benchmark inclusion**: BenchmarkMusicProcessor_Process for performance tracking


## Task 2: Shared Takeout Ingesters Extraction

**Date**: 2026-03-02

### What Was Done

Created `services/core/ingesters/` package to eliminate ~95% duplication between Google Photos and YouTube Music ingesters (766 lines).

**Key Components**:
- `NewServiceIngesters(serviceID, detectionFunc)`: Factory returning 3 ingesters (Folder, ZIP, TGZ)
- `NewServiceFolderDetector(variants)`: Helper to create case-insensitive folder detection
- Shared implementations: folderIngester, zipIngester, tgzIngester
- IngestMulti method: Extracts multiple archives to same destination (previously only in Google Photos)

**Files Created**:
- `services/core/ingesters/ingesters.go` (489 lines)
- `services/core/ingesters/ingesters_test.go` (652 lines)

### Technical Insights

**Parameterization Pattern**:
- ServiceID string prefix: Creates unique IDs like "google-photos-folder", "youtube-music-zip"
- Detection function: Passed to folder ingester for service-specific validation
- Variants array: Supports localized folder names (e.g., "Google Photos", "Google Foto", "Google Fotos")

**Security Features**:
- Zip-slip protection: `filepath.Clean(target)` must start with `filepath.Clean(destDir)+PathSeparator`
- TGZ path traversal: Same protection mechanism for tar archives
- Comprehensive test coverage: `TestZipIngester_ZipSlip`, `TestTGZIngester_Ingest_PathTraversal`

**Implementation Patterns**:
- Context cancellation: All ingesters check `<-ctx.Done()` in extraction loops
- Progress reporting: Pass `core.ProgressFunc` callback, called with (done, total) for each file
- TGZ streams: Total unknown, reports (done, done) since tar archives can't be pre-counted
- IngestMulti: Counts total files first by opening each archive, then extracts

**Test Structure**:
- Factory tests: Verify correct ID prefixing and detector creation
- Ingester tests: CanIngest, Ingest, IngestMulti, context cancellation
- Security tests: Zip-slip and path traversal protection
- Integration test: Demonstrates real-world usage pattern

### Code Quality

**Test Coverage**: 16 tests, all passing
- TestNewServiceIngesters: Factory returns 3 ingesters with proper IDs
- TestNewServiceFolderDetector: Direct match, nested match (3 levels), non-match cases
- TestFolderIngester_*: CanIngest, Ingest with progress, context cancellation
- TestZipIngester_*: CanIngest, Ingest, ZipSlip, IngestMulti, context cancellation
- TestTGZIngester_*: CanIngest, Ingest, PathTraversal, IngestMulti, context cancellation
- TestIntegration_RealWorldUsage: End-to-end service usage pattern

**Build Verification**:
- `go build ./services/core/...` ✓
- `go test -v ./services/core/ingesters/...` ✓ (16/16 passed in 0.012s)
- `go build ./...` ✓ (entire project)

### Design Decisions

**Why unexported types?**
- Services interact via `core.Ingester` interface, not concrete types
- Factory function `NewServiceIngesters()` is the only public API
- Prevents services from creating mismatched ingester instances

**Why detection function instead of interface?**
- Simplest possible API: `func(string) bool`
- Avoids ceremony of defining DetectionStrategy interface
- Helper `NewServiceFolderDetector()` handles common case (folder name variants)

**Why IngestMulti on ZIP/TGZ but not Folder?**
- Folder ingestion is already singular (one directory tree)
- ZIP/TGZ multi-ingestion enables split Takeout archives (e.g., takeout-001.zip, takeout-002.zip)
- Common pattern for large Google Takeout exports

### Lessons Learned

**Test File Generation Gotcha**:
- Initial implementation: `string(rune(i)) + ".txt"` created control characters in paths
- Fixed: `fmt.Sprintf("file%03d.txt", i)` generates valid filenames
- Lesson: Never use raw runes for filename generation in tests

**Context Error Wrapping**:
- Initial test expected unwrapped `context.Canceled`
- Actual: `fmt.Errorf("copy folder: %w", ctx.Err())` wraps error
- Fixed: Check `strings.Contains(err.Error(), "context canceled")`
- Lesson: Always expect wrapped errors from higher-level functions

**Archive Security Best Practice**:
- ALWAYS use `filepath.Clean()` on both target and destDir
- ALWAYS check `strings.HasPrefix(target, destDir+PathSeparator)`
- PathSeparator suffix prevents "/tmp/dest" matching "/tmp/dest-evil"
- Apply to ALL archive formats (ZIP, TGZ, TAR, 7z, etc.)

### Next Steps (Future Tasks)

Tasks 14-15 will migrate existing services to use this shared library:
- Replace `services/googlephotos/ingesters/ingesters.go` (459 lines → ~30 lines wrapper)
- Replace `services/youtubemusic/ingesters/ingesters.go` (307 lines → ~30 lines wrapper)
- Total reduction: 766 lines → ~60 lines (92% decrease)

Services will call:
```go
detector := ingesters.NewServiceFolderDetector([]string{"Google Photos", "Google Foto"})
return ingesters.NewServiceIngesters("google-photos", detector)
```

### Evidence Artifacts

- `.sisyphus/evidence/task-2-shared-ingesters-build.txt`: All 16 tests passing
- `.sisyphus/evidence/task-2-zipslip-protection.txt`: Zip-slip and path traversal protection verified

---

## Task 3: DataType Constants Addition

**Date**: 2026-03-02

### What Was Done

Added 8 new `DataType` constants to `connectors/interfaces.go` to support classification of data from Google services.

**New Constants**:
- `DataTypeEmail = "email"`
- `DataTypeCalendarEvent = "calendar_event"`
- `DataTypeTask = "task"`
- `DataTypeBookmark = "bookmark"`
- `DataTypeLocation = "location"`
- `DataTypeFitnessActivity = "fitness_activity"`
- `DataTypePassword = "password"`
- `DataTypeBrowserHistory = "browser_history"`

**Location**: Lines 42-49 in `connectors/interfaces.go` (added before closing paren of const block)

### Technical Pattern

All DataType constants follow identical pattern:
```go
const (
    DataTypePhoto    DataType = "photo"
    DataTypeVideo    DataType = "video"
    // ... (existing constants)
    DataTypeEmail              DataType = "email"           // NEW
    DataTypeCalendarEvent      DataType = "calendar_event"  // NEW
    // ... (rest of new constants)
)
```

**Naming convention**: `DataType<ServiceName>` in PascalCase
**String values**: snake_case ("calendar_event", not "CalendarEvent")

### Implementation Details

- Constants added to const block (no iota pattern, just static string assignments)
- Each constant uniquely identifies a data category
- Used by processors to classify/tag processed items
- No interfaces or implementation code needed (type constants only)

### Verification

- Build: ✓ `go build ./connectors/...` succeeded
- Diagnostics: ✓ No LSP errors
- Commit: ✓ `feat(connectors): add DataType constants for Google services`
- Evidence: `.sisyphus/evidence/task-3-datatypes.txt`

### Key Learnings

1. **String value convention**: All DataType string values use snake_case, not PascalCase
2. **Alignment pattern**: Existing constants use inconsistent spacing; new constants maintain right-aligned `=` for readability
3. **No breaking changes**: Constants are append-only; existing code unaffected
4. **Build verification**: Always run `go build ./[package]/...` immediately after adding constants to catch any syntax errors

### Why This Matters

These constants enable new service processors (Gmail, Calendar, Tasks, etc.) to properly categorize imported data. The DataType value is stored in `DataItem.Type` field (JSON-serialized as "type") and used for filtering, organization, and targeted processing operations.


---

## [Task 5] Gmail MBOX Processor

**Date**: 2026-03-02

### What Was Done

Created complete Gmail Takeout service (`services/gmail/`) with MBOX parser, .eml extraction, and JSON/EML/CSV outputs (1585 lines).

**Key Components**:
- `metadata/types.go` (162 lines): EmailMessage struct, ParseMboxHeader using net/mail
- `metadata/types_test.go` (202 lines): 7 tests for header parsing, multi-recipients, labels
- `ingesters/ingesters.go` (20 lines): Wrapper using shared ingesters (Mail/Posta detection)
- `processors/processor.go` (637 lines): 6-phase MBOX processing pipeline
- `outputs/outputs.go` (362 lines): 3 outputs (JSON, EML, CSV) with init() registration
- `service.go` (85 lines): Service struct with auto-registration
- `service_test.go` (117 lines): 6 tests for service registration and metadata

**Files Modified**:
- `services/register.go`: Added blank import for gmail service

### Technical Insights

**MBOX Parsing Pattern (RFC 4155)**:
- Messages separated by lines starting with "From " (with space)
- Use bufio.Scanner with 10MB buffer for large messages
- Build message in strings.Builder, parse on delimiter encounter
- Context cancellation checked in scan loop

**Go stdlib Email Parsing**:
- `net/mail.ReadMessage()`: Parses RFC 822 headers from string
- `net/mail.ParseDate()`: Handles various date formats
- `net/mail.ParseAddressList()`: Parses To/CC/BCC with fallback
- `mime.ParseMediaType()`: Extracts Content-Type and boundary
- `mime/multipart.NewReader()`: Iterates attachment parts

**Gmail-Specific Features**:
- X-Gmail-Labels header: Comma-separated label list
- Attachment detection: multipart/mixed or multipart/related
- Label-based organization: Create subdirectories per label

**Import Conflict Resolution**:
- connectors/ package declares `package core` → Import as `conncore "github.com/fulgidus/revoco/connectors"`
- services/core/ imports cleanly → Import as `"github.com/fulgidus/revoco/services/core"`
- Use conncore.DataTypeEmail constant for processed items

**Processor Pipeline (6 phases)**:
1. Scan MBOX files: Walk Mail/ directory, find .mbox files
2. Parse messages: RFC 4155 format, extract headers via net/mail
3. Extract .eml files: Write individual messages to label subdirs
4. Extract metadata: Placeholder for body preview (requires re-parse)
5. Extract attachments: Optional multipart parsing via mime/multipart
6. Write output: library.json with all messages + MBOX metadata

**Output Registration Pattern**:
- Each output struct has init() calling core.RegisterOutput()
- service.go imports `_ "services/gmail/outputs"` to trigger init()
- All 3 outputs auto-register on service import

### Test Coverage

**13 tests, all passing**:
- **Service**: Registration, metadata, ingesters (3), processors, supported outputs, default config
- **Metadata**: ParseMboxHeader, multi-recipients, Gmail labels, attachment detection, address parsing, CSV row/headers

**Test Patterns Used**:
- Synthetic email headers (RFC 822 format strings)
- Edge cases: Empty fields, malformed headers, multiple recipients
- Gmail-specific: X-Gmail-Labels parsing, label organization
- Registry verification: GetService() returns non-nil with correct metadata

### Build Verification

- `go build ./services/gmail/...` ✓ (build successful)
- `go test -v ./services/gmail/...` ✓ (13/13 passed in 0.006s)
- Evidence: `.sisyphus/evidence/task-5-gmail-{build,tests}.txt`

### Design Decisions

**Why Go stdlib only for email parsing?**
- net/mail handles RFC 822/5322 headers robustly
- mime/multipart supports attachment extraction
- No external dependencies needed for basic MBOX processing
- Fallback logic handles edge cases (malformed addresses)

**Why not full-text body extraction?**
- Body parsing requires MIME decoding (quoted-printable, base64)
- Current implementation focuses on metadata + .eml files
- .eml files preserve original message format for external tools
- Future enhancement: Add body preview via content extraction

**Why label-based organization?**
- Gmail labels map to MBOX filenames (Inbox.mbox, Sent.mbox)
- Users expect folder structure matching Gmail's organization
- Single message can have multiple labels (first label used for directory)

**Why 10MB scanner buffer?**
- Large emails with attachments can exceed default 64KB buffer
- 10MB allows processing of most real-world messages
- Alternative: Stream to temp file for >10MB messages (future)

### Lessons Learned

**ParseAddressList angle bracket issue**:
- `addr.String()` returns "Name <email@example.com>" format
- `addr.Address` returns just "email@example.com"
- Fix: Use addr.Address field for clean email extraction
- Tests caught this immediately (expected vs actual mismatch)

**GetService() returns 2 values**:
- Registry pattern changed: `(T, bool)` not just `T`
- Tests broke: `svc := GetService("gmail")` → `svc == nil` check invalid
- Fix: `svc, ok := GetService("gmail"); if !ok { ... }`
- Lesson: Always check registry.go API before writing tests

**Context cancellation placement**:
- Check in outer loops (file iteration, scan loop)
- Don't check in inner loops (header parsing) - too granular
- Use `select { case <-ctx.Done(): return ctx.Err() }`
- Placement: After defer cleanup, before expensive operations

**Import alias necessity**:
- connectors/ declares `package core` (naming quirk)
- services/core/ also exists
- Without alias: "core redeclared in this block" error
- Pattern: `conncore "github.com/fulgidus/revoco/connectors"`

### Next Steps (Future Enhancements)

**Phase 4 body preview** (currently placeholder):
- Re-parse message body during metadata extraction
- Decode quoted-printable / base64 content
- Extract first 200 chars for preview field
- Requires buffering or temp file for large messages

**Attachment naming**:
- Current: `msg_0001_attachment_0.dat`
- Better: Extract Content-Disposition filename parameter
- Handle duplicate filenames (add counter suffix)
- Sanitize filenames (remove special chars)

**EML reconstruction**:
- Current: outputs/outputs.go writes JSON as .eml placeholder
- Correct: Write raw RFC 822 message from MBOX parser
- Requires: Store message body during Phase 2 parsing
- Trade-off: Memory usage vs re-parsing MBOX files

### Evidence Artifacts

- `.sisyphus/evidence/task-5-gmail-build.txt`: Build successful
- `.sisyphus/evidence/task-5-gmail-tests.txt`: 13/13 tests passed


## Task 6: Google Contacts Service (2026-03-02)

### Implementation Summary
Successfully built complete Google Contacts Takeout processor service with vCard parsing.

**Files Created:**
1. `services/contacts/service.go` (86 lines) - Service registration with core.Service interface
2. `services/contacts/metadata/types.go` (410 lines) - Contact structs, ParseVCard, RFC 2426/6350 compliance
3. `services/contacts/ingesters/ingesters.go` (21 lines) - Shared ingesters factory (Contacts/Contatti/Contactos/Kontakte)
4. `services/contacts/processors/processor.go` (494 lines) - 5-phase processing pipeline
5. `services/contacts/outputs/outputs.go` (547 lines) - VCF/JSON/CSV exporters with init() registration
6. `services/contacts/service_test.go` (118 lines) - Registration and metadata tests
7. `services/contacts/metadata/types_test.go` (388 lines) - vCard parsing tests (13 tests)

**Modified:**
- `services/register.go` - Added contacts blank import

**Verification:**
- ✅ All 19 tests pass (6 service tests, 13 metadata tests)
- ✅ Build successful (`go build ./services/contacts/...`)
- ✅ Service registers correctly (`core.GetService("contacts")` works)
- ✅ Total: 2057 lines across 7 Go files

### Technical Patterns Applied

**vCard Parsing (RFC 2426/6350):**
- Line folding: Lines starting with space/tab continue previous line
- Quoted-printable decoding: `mime/quotedprintable` for non-ASCII
- Parameter parsing: `EMAIL;TYPE=HOME:...` → extract TYPE params
- Multi-value properties: Multiple EMAIL/TEL/ADR entries per contact
- Structured values: N property has 5 semicolon-separated components

**Processing Pipeline:**
```
Phase 1: Scan .vcf files
Phase 2: Parse vCards (metadata.ParseVCard)
Phase 3: Extract metadata → ProcessedItems
Phase 4: Normalize data (stats: email/phone/address/photo/org counts)
Phase 5: Generate summary JSON
```

**Output Registration:**
All three outputs (`contacts-vcf`, `contacts-json`, `contacts-csv`) auto-register via `init()` in `outputs.go`. Critical: Service imports outputs package with blank import.

**Shared Ingesters:**
Used `ingesters.NewServiceIngesters("contacts", detector)` with multilingual folder detection (4 language variants).

### Key Learnings

1. **vCard Line Folding Is Tricky:**
   - Must check `strings.HasPrefix(line, " ")` AND `strings.HasPrefix(line, "\t")`
   - Append unfolded text to PREVIOUS line (not current)
   - Store lines in array during parsing to support this

2. **Dual Blank Import Required:**
   - `services/register.go` imports `services/contacts`
   - `services/contacts/service.go` imports `services/contacts/outputs`
   - Without second import, outputs don't register

3. **vCard Property Parameters:**
   - Semicolon-separated: `EMAIL;TYPE=HOME;ENCODING=QUOTED-PRINTABLE:...`
   - Shorthand notation: `TYPE` keyword optional in some cases
   - Must parse recursively for multiple params

4. **Test Coverage:**
   - Single contact, multiple contacts, quoted-printable, line folding
   - Empty input, malformed input (missing END:VCARD)
   - Address parsing (7 semicolon fields)
   - Birthday parsing (multiple formats)
   - Multiple emails with PREF flag → Primary detection
   - Groups/Categories (comma-separated)
   - CSV row generation with nil-safe time formatting

5. **Contact Metadata Extraction:**
   - Store full Contact struct in ProcessedItem.Metadata as `map[string]any`
   - Output adapters reconstruct Contact via JSON marshal/unmarshal
   - DestRelPath: `contacts/{sanitized_uid}.vcf`

### Code Quality Notes

**Strengths:**
- ✅ Context cancellation in all loops
- ✅ Progress reporting in all phases
- ✅ Multilingual folder detection (4 languages)
- ✅ Comprehensive test coverage (19 tests total)
- ✅ RFC-compliant vCard parsing (2426/6350)

**Deferred:**
- Photo extraction (base64 decode) - marked TODO
- Duplicate merging - marked TODO in normalizeData
- Processor tests - no processor_test.go (pattern consistent with gmail)
- Output tests - no outputs_test.go (pattern consistent with gmail)

### Patterns to Reuse

1. **vCard Parsing Template:**
   ```go
   scanner := bufio.NewScanner(reader)
   var currentVCard []string
   for scanner.Scan() {
       line := scanner.Text()
       if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
           // Line folding: append to previous line
           lastIdx := len(currentVCard) - 1
           currentVCard[lastIdx] += strings.TrimPrefix(line, " ")
           continue
       }
       // Process BEGIN/END blocks
   }
   ```

2. **Service Factory with Outputs:**
   ```go
   // service.go
   import (
       _ "package/outputs"  // Critical blank import
   )
   ```

3. **Multilingual Detection:**
   ```go
   detector := ingesters.NewServiceFolderDetector([]string{
       "Contacts", "Contatti", "Contactos", "Kontakte",
   })
   ```

### Follow Gmail Pattern Perfectly

This implementation mirrors Gmail service structure exactly:
- Service struct with ingesters/processors fields
- 5-phase processing pipeline
- Output auto-registration via init()
- Shared ingesters for folder/zip/tgz
- Test structure: registration → metadata → ingesters → processors → outputs → config


---

## [Wave 2 Complete - 2026-03-02] Tasks 6-8: Tier 1 Services

### Services Implemented
**Contacts** (Task 6):
- vCard 2.1/3.0/4.0 parsing (RFC 2426, RFC 6350)
- Line folding, CHARSET handling, quoted-printable decoding
- 3 outputs: VCF (individual + single file), JSON, CSV
- 19 tests passing (6 service + 13 metadata)
- 2,057 lines across 7 files
- Commit: 46b5c76

**Calendar** (Task 7):
- ICS/iCalendar parsing (RFC 5545)
- VEVENT/VCALENDAR handling, line unfolding, RRULE support
- 3 outputs: ICS (clean rebuild), JSON, CSV
- 16 tests passing (5 service + 12 metadata)
- 1,494 lines across 7 files
- Commit: 9a81547
- **Fix applied**: Test expected 3 outputs but service declared 4 (includes "local-folder")

**Keep** (Task 8):
- Keep JSON parsing with checkbox list → Markdown conversion
- Labels, attachments, color, pinned, archived, trashed support
- 3 outputs: Markdown, JSON (batch), HTML (styled)
- 18 tests passing (5 service + 13 metadata)
- 2,030 lines across 7 files
- Commit: 77f4615

### Pattern Consistency
All 3 services follow Gmail pattern exactly:
- Service registration via init() + blank imports
- Shared ingester usage (folder/ZIP/TGZ)
- 7-file structure: service.go, service_test.go, ingesters/, metadata/, processors/, outputs/
- Metadata types implement ALL required methods (11 methods per type)
- Comprehensive test coverage for both service and metadata layers

### Test Insights
**Output declaration pattern**: Services declare "local-folder" in SupportedOutputs() but tests should only verify service-specific outputs are registered (local-folder is a common output, not service-specific).

**Service test pattern**: Always check:
1. Service registration: `core.GetService(id) != nil`
2. Basic metadata: ID(), Name(), Description()
3. Configuration: DefaultConfig() returns valid ServiceConfig
4. Outputs: SupportedOutputs() returns expected count
5. Output registration: Only verify service-specific outputs exist in registry

### Registration Wiring (VERIFIED)
All 3 services correctly wired in `services/register.go`:
- Line 9: `_ "github.com/fulgidus/revoco/services/gmail"` (Task 5)
- Line 10: `_ "github.com/fulgidus/revoco/services/keep"` (Task 8)
- Line 11: `_ "github.com/fulgidus/revoco/services/contacts"` (Task 6)
- Line 12: `_ "github.com/fulgidus/revoco/services/calendar"` (Task 7)

Each service also has internal outputs registration via blank import in service.go.

### Wave 2 Progress
- Completed: 4/4 tasks (Gmail, Contacts, Calendar, Keep)
- Total tests added: 66 (13 Gmail + 19 Contacts + 16 Calendar + 18 Keep)
- Total lines implemented: 7,167 (1,586 Gmail + 2,057 Contacts + 1,494 Calendar + 2,030 Keep)
- All tests passing ✅
- All services building successfully ✅
- All services registered correctly ✅

### Next: Wave 3 (Tier 2 Services)
Tasks 9-11 ready to start:
- Task 9: Google Tasks Takeout processor
- Task 10: Google Maps Takeout processor
- Task 11: Chrome Takeout processor

Pattern established, shared ingesters working perfectly, ready for Wave 3 parallelization.
