// Package schema defines session configuration schemas and migration functions.
//
// This package provides:
//   - Schema definitions for each config version
//   - Version detection from raw JSON
//   - Migration functions between versions
//
// Version history:
//   - v1: Legacy pipeline-based architecture (service + ingester + processor)
//   - v2: Connector-based architecture (connectors + processors)
package schema

import (
	"encoding/json"
	"fmt"
)

// Version represents a session config schema version.
type Version int

const (
	// VersionUnknown indicates the version could not be determined.
	VersionUnknown Version = 0
	// Version1 is the legacy pipeline-based architecture.
	Version1 Version = 1
	// Version2 is the connector-based architecture.
	Version2 Version = 2
	// CurrentVersion is the latest schema version.
	CurrentVersion = Version2
)

// String returns a human-readable version string.
func (v Version) String() string {
	switch v {
	case Version1:
		return "v1 (pipeline-based)"
	case Version2:
		return "v2 (connector-based)"
	default:
		return "unknown"
	}
}

// DetectVersion determines the schema version from raw config JSON.
// It examines the structure to determine which version it matches.
func DetectVersion(data []byte) (Version, error) {
	// First, try to parse with explicit version field
	var versionCheck struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &versionCheck); err != nil {
		return VersionUnknown, fmt.Errorf("schema: invalid JSON: %w", err)
	}

	// Explicit version takes precedence
	if versionCheck.Version >= 2 {
		return Version2, nil
	}

	// Check for v2 indicators (connectors array with items)
	var connectorCheck struct {
		Connectors struct {
			Connectors []json.RawMessage `json:"connectors"`
		} `json:"connectors"`
	}
	if err := json.Unmarshal(data, &connectorCheck); err == nil {
		if len(connectorCheck.Connectors.Connectors) > 0 {
			return Version2, nil
		}
	}

	// Check for v1 indicators (pipeline with service_id)
	var pipelineCheck struct {
		Pipeline struct {
			ServiceID string `json:"service_id"`
		} `json:"pipeline"`
	}
	if err := json.Unmarshal(data, &pipelineCheck); err == nil {
		if pipelineCheck.Pipeline.ServiceID != "" {
			return Version1, nil
		}
	}

	// Default to v1 for backwards compatibility with old configs
	return Version1, nil
}

// NeedsMigration checks if a config needs to be migrated to the current version.
func NeedsMigration(version Version) bool {
	return version < CurrentVersion
}

// MigrationPath returns the sequence of migrations needed to reach the current version.
func MigrationPath(from Version) []Version {
	if from >= CurrentVersion {
		return nil
	}
	var path []Version
	for v := from + 1; v <= CurrentVersion; v++ {
		path = append(path, v)
	}
	return path
}
