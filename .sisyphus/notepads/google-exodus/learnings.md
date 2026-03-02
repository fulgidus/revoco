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

