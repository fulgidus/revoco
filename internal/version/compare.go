package version

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

// IsDevVersion checks if a version tag represents a development version.
// Development versions contain the "-dev-" substring in their tag.
func IsDevVersion(tag string) bool {
	return strings.Contains(tag, "-dev-")
}

// ParseVersion parses a version string into a semver.Version object.
// It automatically strips the "v" prefix if present and returns an error
// if the version string is invalid according to semantic versioning rules.
func ParseVersion(tag string) (*semver.Version, error) {
	// Strip "v" prefix if present
	tag = strings.TrimPrefix(tag, "v")

	// Parse using Masterminds/semver
	return semver.NewVersion(tag)
}

// IsNewer compares two version strings and returns true if the candidate
// version is newer than the current version. Both versions must be valid
// semantic versions.
//
// Version comparison rules:
//   - Stable versions are compared using standard semver comparison
//   - Dev versions with higher base versions are considered newer
//   - Dev versions with the same base version are compared by their
//     prerelease identifiers (which include timestamps), so newer timestamps
//     result in lexicographically greater prerelease strings
func IsNewer(candidate, current string) (bool, error) {
	candidateVer, err := ParseVersion(candidate)
	if err != nil {
		return false, err
	}

	currentVer, err := ParseVersion(current)
	if err != nil {
		return false, err
	}

	// Use semver's GreaterThan method which handles prerelease versions correctly.
	// According to semver spec, prerelease versions are compared lexicographically,
	// so "1.0.0-dev-2026-03-05T11-00-00" > "1.0.0-dev-2026-03-05T10-00-00"
	return candidateVer.GreaterThan(currentVer), nil
}
