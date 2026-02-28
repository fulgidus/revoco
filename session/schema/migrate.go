package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Migrate transforms a session config from one version to the next.
// Returns the migrated config as JSON bytes.
func Migrate(data []byte, from, to Version) ([]byte, error) {
	if from >= to {
		return data, nil // No migration needed
	}

	var result []byte = data
	var err error

	// Apply migrations in sequence
	for v := from; v < to; v++ {
		switch v {
		case Version1:
			result, err = migrateV1toV2(result)
		default:
			return nil, fmt.Errorf("schema: no migration path from %s to %s", v, v+1)
		}
		if err != nil {
			return nil, fmt.Errorf("schema: migration %s -> %s failed: %w", v, v+1, err)
		}
	}

	return result, nil
}

// MigrateToLatest migrates a config to the current version.
func MigrateToLatest(data []byte) ([]byte, Version, error) {
	from, err := DetectVersion(data)
	if err != nil {
		return nil, VersionUnknown, err
	}

	if from >= CurrentVersion {
		return data, from, nil
	}

	migrated, err := Migrate(data, from, CurrentVersion)
	if err != nil {
		return nil, from, err
	}

	return migrated, CurrentVersion, nil
}

// migrateV1toV2 converts a v1 config to v2 format.
func migrateV1toV2(data []byte) ([]byte, error) {
	var v1 ConfigV1
	if err := json.Unmarshal(data, &v1); err != nil {
		return nil, fmt.Errorf("parse v1 config: %w", err)
	}

	v2 := ConfigV2{
		Name:               v1.Name,
		Created:            v1.Created,
		Updated:            time.Now(), // Update timestamp since we're migrating
		OutputDir:          v1.OutputDir,
		UseMove:            v1.UseMove,
		DryRun:             v1.DryRun,
		Recover:            v1.Recover,
		Status:             v1.Status,
		LastPhaseCompleted: v1.LastPhaseCompleted,
		LastError:          v1.LastError,
		Version:            2,
		Source: SourceV2{
			Type:         v1.Source.Type,
			OriginalPath: v1.Source.OriginalPath,
			ImportedPath: v1.Source.ImportedPath,
		},
		Pipeline: &v1.Pipeline, // Keep reference to old pipeline for debugging
		Connectors: ConnectorsConfig{
			Connectors:        make([]ConnectorConfigV2, 0),
			ParallelRetrieval: true,
			AutoProcess:       false,
		},
	}

	// Convert v1 source to a connector
	if v1.Source.OriginalPath != "" || v1.Source.ImportedPath != "" {
		sourceConn := convertSourceToConnector(v1.Source, v1.Pipeline)
		if sourceConn != nil {
			v2.Connectors.Connectors = append(v2.Connectors.Connectors, *sourceConn)
		}
	}

	// Convert v1 outputs to connectors
	for _, out := range v1.Pipeline.OutputSettings {
		outConn := convertOutputToConnector(out)
		if outConn != nil {
			v2.Connectors.Connectors = append(v2.Connectors.Connectors, *outConn)
		}
	}

	return json.MarshalIndent(v2, "", "  ")
}

// convertSourceToConnector creates a connector config from a v1 source.
func convertSourceToConnector(source SourceV1, pipeline PipelineConfig) *ConnectorConfigV2 {
	// Determine connector type from source type
	var connectorID string
	switch source.Type {
	case "folder":
		connectorID = "local-folder"
	case "zip":
		// Check if it's multiple zips (comma-separated)
		if strings.Contains(source.OriginalPath, ",") {
			connectorID = "local-multi-zip"
		} else {
			connectorID = "local-zip"
		}
	case "tgz":
		if strings.Contains(source.OriginalPath, ",") {
			connectorID = "local-multi-tgz"
		} else {
			connectorID = "local-tgz"
		}
	default:
		connectorID = "local-folder"
	}

	// Determine import mode from old settings
	importMode := "copy"
	if settings := pipeline.ProcessorSettings; settings != nil {
		if useMove, ok := settings["use_move"].(bool); ok && useMove {
			importMode = "move"
		}
	}

	// Build settings
	settings := make(map[string]any)
	if source.ImportedPath != "" {
		settings["path"] = source.ImportedPath
	} else if source.OriginalPath != "" {
		// Handle multi-path case
		if strings.Contains(source.OriginalPath, ",") {
			settings["paths"] = strings.Split(source.OriginalPath, ",")
		} else {
			settings["path"] = source.OriginalPath
		}
	}

	// Create a descriptive name
	name := "Imported Source"
	if source.OriginalPath != "" {
		parts := strings.Split(source.OriginalPath, ",")
		if len(parts) == 1 {
			// Get just the filename/dirname
			path := source.OriginalPath
			if idx := strings.LastIndex(path, "/"); idx >= 0 {
				path = path[idx+1:]
			}
			if idx := strings.LastIndex(path, "\\"); idx >= 0 {
				path = path[idx+1:]
			}
			name = path
		} else {
			name = fmt.Sprintf("%d archives", len(parts))
		}
	}

	return &ConnectorConfigV2{
		ConnectorID: connectorID,
		InstanceID:  "migrated-source-" + uuid.New().String()[:8],
		Name:        name,
		Roles:       ConnectorRolesV2{IsInput: true},
		ImportMode:  importMode,
		Settings:    settings,
		Enabled:     true,
	}
}

// convertOutputToConnector creates a connector config from a v1 output setting.
func convertOutputToConnector(out OutputSettingV1) *ConnectorConfigV2 {
	// Map v1 output IDs to v2 connector IDs
	var connectorID string
	switch out.OutputID {
	case "local-folder", "folder":
		connectorID = "local-folder"
	default:
		// For unknown outputs, try to use the ID directly
		connectorID = out.OutputID
	}

	return &ConnectorConfigV2{
		ConnectorID: connectorID,
		InstanceID:  "migrated-output-" + uuid.New().String()[:8],
		Name:        "Migrated Output",
		Roles:       ConnectorRolesV2{IsOutput: true},
		Settings:    out.Config,
		Enabled:     true,
	}
}
