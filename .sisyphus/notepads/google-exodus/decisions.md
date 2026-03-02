# Architectural Decisions — Google Exodus

> Key design choices and their rationale

---

## [INIT] Session: ses_353ba6eb1ffeGwDKcQ5pCzqD0Y

### Service Pattern
- Following established pattern: service.go + ingesters/ + processors/ + outputs/ + metadata/
- Services auto-register via `init()` functions
- Shared ingester library in `services/core/ingesters/` to eliminate duplication

### Testing Strategy
- Tests after implementation (not TDD)
- Go standard `testing` package only
- Synthetic Takeout fixtures for each service
- Evidence files in `.sisyphus/evidence/task-{N}-{scenario}.txt`

### API Design Decisions
- Shared ingester factory: `ingesters.NewServiceIngesters(serviceID, detectionFunc)`
- Folder detector helper: `ingesters.NewServiceFolderDetector(variants []string)`
- Functions live in `services/core/ingesters/` package (imported as `ingesters "..."`)

---

## Task 2: Shared Ingester Package Design

**Date**: 2026-03-02

### API Design Choice: Factory Function vs Registry

**Decision**: Use factory function `NewServiceIngesters()` returning slice of `core.Ingester`

**Alternatives Considered**:
1. Registry pattern (like connectors): `ingester.Register("google-photos-folder", constructor)`
2. Builder pattern: `NewServiceIngesterBuilder().WithID("google-photos").WithDetector(fn).Build()`
3. Individual constructors: `NewFolderIngester()`, `NewZipIngester()`, `NewTGZIngester()` with parameters

**Rationale**:
- Services need ALL THREE ingesters (folder, zip, tgz) - factory returns complete set
- Registry adds complexity (global state, init() side effects, discovery API)
- Builder adds verbosity for no real benefit (services always need same 3 types)
- Individual constructors require services to know implementation details

**Trade-offs**:
- ✓ Simple API: One function call returns everything needed
- ✓ No global state: Pure function with explicit dependencies
- ✓ Type-safe: Returns `[]core.Ingester`, not interface{}
- ✗ Less flexible: Can't register custom ingester types (acceptable - services can extend slice)

### Parameterization: Detection Function vs Interface

**Decision**: Pass `func(string) bool` for folder detection

**Alternatives Considered**:
1. Interface: `type Detector interface { CanDetect(path string) bool }`
2. Struct with variants: `type DetectorConfig struct { Variants []string }`
3. Hard-coded service name: `NewServiceIngesters("google-photos")` with internal lookup

**Rationale**:
- Function is simplest Go idiom: no ceremony, no boilerplate
- Helper `NewServiceFolderDetector()` covers 95% case (folder name variants)
- Advanced services can pass custom function (e.g., check for specific files)
- Interface adds no value (single method, no polymorphism needed)

**Trade-offs**:
- ✓ Flexible: Services can implement any detection logic
- ✓ Testable: Easy to pass lambda for tests (`func(string) bool { return true }`)
- ✓ Discoverable: Helper function documents common pattern
- ✗ Less discoverable: No autocomplete for detector creation (mitigated by godoc example)

### Type Visibility: Unexported Ingesters

**Decision**: Keep folderIngester, zipIngester, tgzIngester unexported

**Rationale**:
- Services interact via `core.Ingester` interface, not concrete types
- Factory function is the only intended creation point
- Prevents services from instantiating mismatched ingesters (e.g., wrong serviceID)
- Forces consistent usage pattern across all services

**Trade-offs**:
- ✓ Encapsulation: Implementation details hidden
- ✓ Consistency: Services can't create non-standard ingesters
- ✓ Refactorable: Can change implementation without breaking services
- ✗ Less flexible: Services can't extend individual ingester types (rare need)

### IngestMulti Method Placement

**Decision**: Add IngestMulti to ZIP and TGZ ingesters

**Rationale**:
- Google Takeout splits large exports into multiple archives (takeout-001.zip, takeout-002.zip, ...)
- YouTube Music will need this feature (not yet implemented in current code)
- More efficient than calling Ingest() multiple times (single progress count, single destDir setup)
- Folder ingester doesn't need it (folders aren't split)

**Trade-offs**:
- ✓ Performance: Single file count pass across all archives
- ✓ UX: One progress bar for entire multi-archive extraction
- ✓ Atomicity: All-or-nothing extraction (context cancellation stops entire operation)
- ✗ API inconsistency: Not all ingesters have IngestMulti (acceptable - folder ingestion is different)

### Security: Zip-Slip Protection Implementation

**Decision**: Use `filepath.Clean()` + `strings.HasPrefix()` check with PathSeparator suffix

**Rationale**:
- `filepath.Clean()` normalizes path separators and removes ".." segments
- `strings.HasPrefix()` ensures target is under destDir
- PathSeparator suffix prevents prefix attacks: "/tmp/dest" vs "/tmp/dest-evil"
- Applied consistently to ZIP and TGZ formats

**Alternatives Considered**:
1. `filepath.Rel()` + check for ".." in result (more complex, same security)
2. Symlink detection (overkill - archives can't create symlinks in this implementation)
3. Chroot/jail (OS-specific, requires root, not portable)

**Trade-offs**:
- ✓ Simple: Two function calls, easy to audit
- ✓ Fast: No filesystem operations, pure string checks
- ✓ Portable: Works on all platforms
- ✓ Comprehensive test coverage: Malicious ZIP/TGZ test cases
- ✗ No symlink protection: Acceptable - Go's zip/tar don't preserve symlinks by default

