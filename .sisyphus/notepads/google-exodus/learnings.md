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
