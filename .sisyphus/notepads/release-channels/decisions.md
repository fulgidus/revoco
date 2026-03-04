
## Task 3: Create .goreleaser.dev.yaml for Dev Channel Releases

**Date:** 2026-03-04
**Status:** COMPLETED

### Decision: Package Manager Exclusion Strategy
- **Why:** Dev channel releases should not publish to stable package managers (brew, scoop, AUR, nfpm, winget)
- **How:** Created separate `.goreleaser.dev.yaml` config file with all package manager sections removed
- **Rationale:** Prevents accidental publication of pre-release versions to end-user package managers

### Implementation Details
1. Copied `.goreleaser.yaml` as template
2. Removed sections:
   - `brews:` (Homebrew tap)
   - `scoops:` (Scoop bucket)
   - `aurs:` (Arch Linux AUR)
   - `nfpms:` (Debian/RPM packages)
   - `winget:` (Windows package manager)

3. Kept sections:
   - `builds:` (cross-platform Go builds)
   - `archives:` (tarball/zip archives)
   - `checksum:` (SHA256 checksums)
   - `changelog:` (GitHub-integrated changelog)
   - `dockers:` (container images)
   - `release:` (GitHub release page)

4. Updated Docker image templates:
   - Tag-based: `ghcr.io/fulgidus/revoco:{{ .Tag }}`
   - Floating dev tag: `ghcr.io/fulgidus/revoco:dev`

5. Set `release.prerelease: auto` to mark releases as pre-releases based on version format

### Validation Results
- ✓ YAML syntax valid (Python yaml.safe_load)
- ✓ All required sections present
- ✓ Docker tags correct (tag-specific + floating `:dev`)
- ✓ Prerelease setting applied (`prerelease: auto`)
- ✓ No package manager sections found (grep verification)
- ✓ File size: 134 lines (vs 211 in full config — 37% reduction)

### Key Insight
The dev channel config is 37% smaller because it removes all package manager publishing infrastructure. This reduces attack surface and prevents accidental publication to stable channels. The floating `:dev` tag allows container users to always get the latest development version with `--pull`.

## Task 4: CI Dev Pipeline and Release Protection

**Date**: 2026-03-04

**Decision**: Created `.github/workflows/ci-dev.yaml` for automated develop branch releases with timestamp+SHA-based dev tags, and protected `release.yml` from dev tag triggers.

**Context**: 
- Develop branch needs automated dev releases for continuous deployment testing
- Dev tags must not trigger stable release pipeline (which publishes to package managers)
- Tag collision risk with timestamp-only scheme required commit SHA inclusion

**Implementation**:
1. **ci-dev.yaml Structure**:
   - Trigger: `push` to `develop` branch (NOT tags)
   - Tag generation: `vX.Y.Z-dev-YYYY-MM-DDThh-mm-ss-SHORT_SHA` format
   - Uses `.goreleaser.dev.yaml` config (no snapshot flag)
   - Permissions: `contents:write`, `packages:write`
   - Concurrency group: `dev-release` with `cancel-in-progress: false`
   - Sets `GORELEASER_CURRENT_TAG` env var for goreleaser

2. **release.yml Protection**:
   - Modified tag filter from `'v*'` to `['v*', '!v*-dev-*']`
   - Negative pattern `!v*-dev-*` excludes all dev tags
   - Prevents accidental package manager publishing on dev releases

3. **Tag Format Rationale**:
   - Base: Latest stable tag from `git describe --tags --abbrev=0`
   - Suffix: `-dev-YYYY-MM-DDThh-mm-ss-SHORT_SHA`
   - UTC timestamp ensures chronological ordering
   - Short SHA prevents collision within same second
   - Format: `v0.1.10-dev-2026-03-04T21-30-45-a1b2c3d`

**Validation**:
- Both YAML files validate with python3 yaml.safe_load()
- Grep confirms: develop trigger, .goreleaser.dev.yaml reference, no --snapshot
- release.yml correctly excludes dev tags with `!v*-dev-*` pattern

**Evidence**: `.sisyphus/evidence/task-4-*.txt` (6 verification files)
