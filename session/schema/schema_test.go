package schema

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Version Tests
// ============================================================================

func TestVersion_String(t *testing.T) {
	tests := []struct {
		version  Version
		expected string
	}{
		{VersionUnknown, "unknown"},
		{Version1, "v1 (pipeline-based)"},
		{Version2, "v2 (connector-based)"},
		{Version(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.version.String()
		if got != tt.expected {
			t.Errorf("Version(%d).String() = %q, want %q", tt.version, got, tt.expected)
		}
	}
}

func TestCurrentVersion(t *testing.T) {
	if CurrentVersion != Version2 {
		t.Errorf("CurrentVersion = %v, want %v", CurrentVersion, Version2)
	}
}

// ============================================================================
// DetectVersion Tests
// ============================================================================

func TestDetectVersion_InvalidJSON(t *testing.T) {
	_, err := DetectVersion([]byte("not valid json"))
	if err == nil {
		t.Error("DetectVersion() should return error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error should mention 'invalid JSON', got: %v", err)
	}
}

func TestDetectVersion_ExplicitVersion2(t *testing.T) {
	data := []byte(`{"version": 2, "name": "test"}`)
	version, err := DetectVersion(data)
	if err != nil {
		t.Fatalf("DetectVersion() error = %v", err)
	}
	if version != Version2 {
		t.Errorf("DetectVersion() = %v, want %v", version, Version2)
	}
}

func TestDetectVersion_ExplicitVersion3Plus(t *testing.T) {
	// Future versions should be detected as v2 (latest known)
	data := []byte(`{"version": 3, "name": "test"}`)
	version, err := DetectVersion(data)
	if err != nil {
		t.Fatalf("DetectVersion() error = %v", err)
	}
	if version != Version2 {
		t.Errorf("DetectVersion() = %v, want %v for future version", version, Version2)
	}
}

func TestDetectVersion_V2ByConnectors(t *testing.T) {
	data := []byte(`{
		"name": "test",
		"connectors": {
			"connectors": [
				{"connector_id": "local-folder", "instance_id": "abc123"}
			]
		}
	}`)
	version, err := DetectVersion(data)
	if err != nil {
		t.Fatalf("DetectVersion() error = %v", err)
	}
	if version != Version2 {
		t.Errorf("DetectVersion() = %v, want %v", version, Version2)
	}
}

func TestDetectVersion_V1ByPipeline(t *testing.T) {
	data := []byte(`{
		"name": "test",
		"pipeline": {
			"service_id": "whatsapp"
		}
	}`)
	version, err := DetectVersion(data)
	if err != nil {
		t.Fatalf("DetectVersion() error = %v", err)
	}
	if version != Version1 {
		t.Errorf("DetectVersion() = %v, want %v", version, Version1)
	}
}

func TestDetectVersion_DefaultsToV1(t *testing.T) {
	// Empty or minimal config should default to v1 for backwards compat
	data := []byte(`{"name": "test"}`)
	version, err := DetectVersion(data)
	if err != nil {
		t.Fatalf("DetectVersion() error = %v", err)
	}
	if version != Version1 {
		t.Errorf("DetectVersion() = %v, want %v (default)", version, Version1)
	}
}

func TestDetectVersion_EmptyConnectorsIsV1(t *testing.T) {
	// Empty connectors array should still be v1 (no actual connectors configured)
	data := []byte(`{
		"name": "test",
		"connectors": {
			"connectors": []
		}
	}`)
	version, err := DetectVersion(data)
	if err != nil {
		t.Fatalf("DetectVersion() error = %v", err)
	}
	if version != Version1 {
		t.Errorf("DetectVersion() = %v, want %v for empty connectors", version, Version1)
	}
}

// ============================================================================
// NeedsMigration Tests
// ============================================================================

func TestNeedsMigration(t *testing.T) {
	tests := []struct {
		version  Version
		expected bool
	}{
		{VersionUnknown, true},
		{Version1, true},
		{Version2, false},
		{Version(3), false}, // Future version doesn't need migration
	}

	for _, tt := range tests {
		got := NeedsMigration(tt.version)
		if got != tt.expected {
			t.Errorf("NeedsMigration(%v) = %v, want %v", tt.version, got, tt.expected)
		}
	}
}

// ============================================================================
// MigrationPath Tests
// ============================================================================

func TestMigrationPath(t *testing.T) {
	tests := []struct {
		from     Version
		expected []Version
	}{
		{VersionUnknown, []Version{Version1, Version2}},
		{Version1, []Version{Version2}},
		{Version2, nil},
		{Version(3), nil}, // Future version needs no migration
	}

	for _, tt := range tests {
		got := MigrationPath(tt.from)
		if len(got) != len(tt.expected) {
			t.Errorf("MigrationPath(%v) length = %d, want %d", tt.from, len(got), len(tt.expected))
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("MigrationPath(%v)[%d] = %v, want %v", tt.from, i, got[i], tt.expected[i])
			}
		}
	}
}

// ============================================================================
// V1 Schema Tests
// ============================================================================

func TestConfigV1_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	original := ConfigV1{
		Name:      "Test Session",
		Created:   now,
		Updated:   now,
		OutputDir: "/output",
		UseMove:   true,
		DryRun:    false,
		Status:    "ready",
		Source: SourceV1{
			Type:         "folder",
			OriginalPath: "/data/input",
			ImportedPath: "/session/data",
		},
		Pipeline: PipelineConfig{
			ServiceID:  "whatsapp",
			IngesterID: "android",
			ProcessorSettings: map[string]any{
				"use_move": true,
			},
			OutputSettings: []OutputSettingV1{
				{
					OutputID: "local-folder",
					Config: map[string]any{
						"path": "/output",
					},
				},
			},
		},
		Recover: RecoverSettings{
			Concurrency: 4,
			MaxRetry:    3,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ConfigV1
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify key fields
	if parsed.Name != original.Name {
		t.Errorf("Name = %q, want %q", parsed.Name, original.Name)
	}
	if parsed.Source.Type != original.Source.Type {
		t.Errorf("Source.Type = %q, want %q", parsed.Source.Type, original.Source.Type)
	}
	if parsed.Pipeline.ServiceID != original.Pipeline.ServiceID {
		t.Errorf("Pipeline.ServiceID = %q, want %q", parsed.Pipeline.ServiceID, original.Pipeline.ServiceID)
	}
}

// ============================================================================
// V2 Schema Tests
// ============================================================================

func TestConfigV2_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	original := ConfigV2{
		Name:      "Test Session V2",
		Created:   now,
		Updated:   now,
		Version:   2,
		OutputDir: "/output",
		Status:    "ready",
		Connectors: ConnectorsConfig{
			Connectors: []ConnectorConfigV2{
				{
					ConnectorID: "local-folder",
					InstanceID:  "inst-12345",
					Name:        "Input Folder",
					Roles:       ConnectorRolesV2{IsInput: true},
					ImportMode:  "copy",
					Settings: map[string]any{
						"path": "/data/input",
					},
					Enabled: true,
				},
				{
					ConnectorID: "local-folder",
					InstanceID:  "inst-67890",
					Name:        "Output Folder",
					Roles:       ConnectorRolesV2{IsOutput: true},
					Settings: map[string]any{
						"path": "/data/output",
					},
					Enabled: true,
				},
			},
			ParallelRetrieval: true,
			AutoProcess:       false,
			DetectedDataTypes: []string{"chat", "media"},
			Stats: &DataStats{
				TotalItems: 100,
				ByType: map[string]int{
					"chat":  80,
					"media": 20,
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ConfigV2
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify key fields
	if parsed.Name != original.Name {
		t.Errorf("Name = %q, want %q", parsed.Name, original.Name)
	}
	if parsed.Version != 2 {
		t.Errorf("Version = %d, want 2", parsed.Version)
	}
	if len(parsed.Connectors.Connectors) != 2 {
		t.Errorf("Connectors count = %d, want 2", len(parsed.Connectors.Connectors))
	}
	if !parsed.Connectors.Connectors[0].Roles.IsInput {
		t.Error("First connector should have IsInput = true")
	}
	if parsed.Connectors.Stats.TotalItems != 100 {
		t.Errorf("Stats.TotalItems = %d, want 100", parsed.Connectors.Stats.TotalItems)
	}
}

func TestConnectorConfigV2_Roles(t *testing.T) {
	tests := []struct {
		name  string
		roles ConnectorRolesV2
	}{
		{"input only", ConnectorRolesV2{IsInput: true}},
		{"output only", ConnectorRolesV2{IsOutput: true}},
		{"fallback only", ConnectorRolesV2{IsFallback: true}},
		{"input+output", ConnectorRolesV2{IsInput: true, IsOutput: true}},
		{"input+fallback", ConnectorRolesV2{IsInput: true, IsFallback: true}},
		{"all roles", ConnectorRolesV2{IsInput: true, IsOutput: true, IsFallback: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := ConnectorConfigV2{
				ConnectorID: "test",
				InstanceID:  "test-123",
				Roles:       tt.roles,
				Enabled:     true,
			}

			data, err := json.Marshal(conn)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var parsed ConnectorConfigV2
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if parsed.Roles != tt.roles {
				t.Errorf("Roles = %+v, want %+v", parsed.Roles, tt.roles)
			}
		})
	}
}

func TestConnectorConfigV2_FallbackFor(t *testing.T) {
	conn := ConnectorConfigV2{
		ConnectorID: "api-connector",
		InstanceID:  "fallback-123",
		Roles:       ConnectorRolesV2{IsFallback: true},
		FallbackFor: []string{"inst-001", "inst-002"},
		Enabled:     true,
	}

	data, err := json.Marshal(conn)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ConnectorConfigV2
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(parsed.FallbackFor) != 2 {
		t.Errorf("FallbackFor length = %d, want 2", len(parsed.FallbackFor))
	}
}

// ============================================================================
// Migration Tests
// ============================================================================

func TestMigrate_V1toV2_Basic(t *testing.T) {
	v1Config := ConfigV1{
		Name:      "Test Migration",
		Created:   time.Now(),
		Updated:   time.Now(),
		OutputDir: "/output",
		Status:    "ready",
		Source: SourceV1{
			Type:         "folder",
			OriginalPath: "/data/input",
		},
		Pipeline: PipelineConfig{
			ServiceID:  "whatsapp",
			IngesterID: "android",
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal v1 config error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal v2 config error: %v", err)
	}

	// Verify migration results
	if v2Config.Version != 2 {
		t.Errorf("Version = %d, want 2", v2Config.Version)
	}
	if v2Config.Name != v1Config.Name {
		t.Errorf("Name = %q, want %q", v2Config.Name, v1Config.Name)
	}
	if v2Config.OutputDir != v1Config.OutputDir {
		t.Errorf("OutputDir = %q, want %q", v2Config.OutputDir, v1Config.OutputDir)
	}

	// Should have converted source to a connector
	if len(v2Config.Connectors.Connectors) < 1 {
		t.Error("Expected at least one connector from source migration")
	}

	// First connector should be the migrated source
	sourceConn := v2Config.Connectors.Connectors[0]
	if !sourceConn.Roles.IsInput {
		t.Error("Source connector should have IsInput = true")
	}
	if sourceConn.ConnectorID != "local-folder" {
		t.Errorf("Source connector ID = %q, want 'local-folder'", sourceConn.ConnectorID)
	}

	// Pipeline should be kept for reference
	if v2Config.Pipeline == nil {
		t.Error("Pipeline should be preserved for reference")
	}
}

func TestMigrate_V1toV2_WithOutputs(t *testing.T) {
	v1Config := ConfigV1{
		Name: "Test With Outputs",
		Source: SourceV1{
			Type:         "folder",
			OriginalPath: "/input",
		},
		Pipeline: PipelineConfig{
			ServiceID: "whatsapp",
			OutputSettings: []OutputSettingV1{
				{
					OutputID: "local-folder",
					Config: map[string]any{
						"path": "/output1",
					},
				},
				{
					OutputID: "folder",
					Config: map[string]any{
						"path": "/output2",
					},
				},
			},
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Should have 1 input + 2 outputs = 3 connectors
	if len(v2Config.Connectors.Connectors) != 3 {
		t.Errorf("Connectors count = %d, want 3", len(v2Config.Connectors.Connectors))
	}

	// Count by role
	inputCount := 0
	outputCount := 0
	for _, c := range v2Config.Connectors.Connectors {
		if c.Roles.IsInput {
			inputCount++
		}
		if c.Roles.IsOutput {
			outputCount++
		}
	}

	if inputCount != 1 {
		t.Errorf("Input connectors = %d, want 1", inputCount)
	}
	if outputCount != 2 {
		t.Errorf("Output connectors = %d, want 2", outputCount)
	}
}

func TestMigrate_V1toV2_ZipSource(t *testing.T) {
	v1Config := ConfigV1{
		Name: "Zip Source",
		Source: SourceV1{
			Type:         "zip",
			OriginalPath: "/data/archive.zip",
		},
		Pipeline: PipelineConfig{
			ServiceID: "whatsapp",
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(v2Config.Connectors.Connectors) < 1 {
		t.Fatal("Expected at least one connector")
	}

	sourceConn := v2Config.Connectors.Connectors[0]
	if sourceConn.ConnectorID != "local-zip" {
		t.Errorf("ConnectorID = %q, want 'local-zip'", sourceConn.ConnectorID)
	}
}

func TestMigrate_V1toV2_MultiZipSource(t *testing.T) {
	v1Config := ConfigV1{
		Name: "Multi-Zip Source",
		Source: SourceV1{
			Type:         "zip",
			OriginalPath: "/data/archive1.zip,/data/archive2.zip",
		},
		Pipeline: PipelineConfig{
			ServiceID: "whatsapp",
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(v2Config.Connectors.Connectors) < 1 {
		t.Fatal("Expected at least one connector")
	}

	sourceConn := v2Config.Connectors.Connectors[0]
	if sourceConn.ConnectorID != "local-multi-zip" {
		t.Errorf("ConnectorID = %q, want 'local-multi-zip'", sourceConn.ConnectorID)
	}

	// Should have "2 archives" as name
	if !strings.Contains(sourceConn.Name, "archives") {
		t.Errorf("Name = %q, expected to contain 'archives'", sourceConn.Name)
	}
}

func TestMigrate_V1toV2_TgzSource(t *testing.T) {
	v1Config := ConfigV1{
		Name: "TGZ Source",
		Source: SourceV1{
			Type:         "tgz",
			OriginalPath: "/data/archive.tar.gz",
		},
		Pipeline: PipelineConfig{},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(v2Config.Connectors.Connectors) < 1 {
		t.Fatal("Expected at least one connector")
	}

	sourceConn := v2Config.Connectors.Connectors[0]
	if sourceConn.ConnectorID != "local-tgz" {
		t.Errorf("ConnectorID = %q, want 'local-tgz'", sourceConn.ConnectorID)
	}
}

func TestMigrate_V1toV2_UseMoveImportMode(t *testing.T) {
	v1Config := ConfigV1{
		Name: "Move Mode",
		Source: SourceV1{
			Type:         "folder",
			OriginalPath: "/input",
		},
		Pipeline: PipelineConfig{
			ProcessorSettings: map[string]any{
				"use_move": true,
			},
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(v2Config.Connectors.Connectors) < 1 {
		t.Fatal("Expected at least one connector")
	}

	sourceConn := v2Config.Connectors.Connectors[0]
	if sourceConn.ImportMode != "move" {
		t.Errorf("ImportMode = %q, want 'move'", sourceConn.ImportMode)
	}
}

func TestMigrate_NoMigrationNeeded(t *testing.T) {
	original := []byte(`{"version": 2, "name": "already v2"}`)

	result, err := Migrate(original, Version2, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	if string(result) != string(original) {
		t.Error("Expected no change when from >= to")
	}
}

func TestMigrate_InvalidJSON(t *testing.T) {
	_, err := Migrate([]byte("invalid"), Version1, Version2)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// ============================================================================
// MigrateToLatest Tests
// ============================================================================

func TestMigrateToLatest_V1Config(t *testing.T) {
	v1Config := ConfigV1{
		Name: "V1 Config",
		Source: SourceV1{
			Type:         "folder",
			OriginalPath: "/input",
		},
		Pipeline: PipelineConfig{
			ServiceID: "whatsapp",
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	migratedData, version, err := MigrateToLatest(v1Data)
	if err != nil {
		t.Fatalf("MigrateToLatest() error: %v", err)
	}

	if version != CurrentVersion {
		t.Errorf("Version = %v, want %v", version, CurrentVersion)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(migratedData, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if v2Config.Version != 2 {
		t.Errorf("Migrated Version = %d, want 2", v2Config.Version)
	}
}

func TestMigrateToLatest_AlreadyLatest(t *testing.T) {
	v2Config := ConfigV2{
		Name:    "V2 Config",
		Version: 2,
	}

	v2Data, err := json.Marshal(v2Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	migratedData, version, err := MigrateToLatest(v2Data)
	if err != nil {
		t.Fatalf("MigrateToLatest() error: %v", err)
	}

	if version != Version2 {
		t.Errorf("Version = %v, want %v", version, Version2)
	}

	// Data should be returned as-is
	var parsed ConfigV2
	if err := json.Unmarshal(migratedData, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.Name != v2Config.Name {
		t.Errorf("Name = %q, want %q", parsed.Name, v2Config.Name)
	}
}

func TestMigrateToLatest_InvalidJSON(t *testing.T) {
	_, _, err := MigrateToLatest([]byte("not json"))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// ============================================================================
// DataStats Tests
// ============================================================================

func TestDataStats_JSONRoundTrip(t *testing.T) {
	original := DataStats{
		TotalItems: 500,
		ByType: map[string]int{
			"chat":   300,
			"media":  150,
			"status": 50,
		},
		ByConnector: map[string]int{
			"conn-1": 400,
			"conn-2": 100,
		},
		TotalSize:     1024 * 1024 * 100, // 100MB
		Duplicates:    25,
		Missing:       10,
		Repairable:    8,
		ProcessedOK:   465,
		ProcessedFail: 10,
		Errors:        []string{"error1", "error2"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed DataStats
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.TotalItems != original.TotalItems {
		t.Errorf("TotalItems = %d, want %d", parsed.TotalItems, original.TotalItems)
	}
	if parsed.TotalSize != original.TotalSize {
		t.Errorf("TotalSize = %d, want %d", parsed.TotalSize, original.TotalSize)
	}
	if len(parsed.ByType) != len(original.ByType) {
		t.Errorf("ByType length = %d, want %d", len(parsed.ByType), len(original.ByType))
	}
	if len(parsed.Errors) != 2 {
		t.Errorf("Errors length = %d, want 2", len(parsed.Errors))
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestMigrate_EmptySource(t *testing.T) {
	v1Config := ConfigV1{
		Name:   "No Source",
		Source: SourceV1{}, // Empty source
		Pipeline: PipelineConfig{
			ServiceID: "whatsapp",
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Should have no connectors (empty source shouldn't create one)
	if len(v2Config.Connectors.Connectors) != 0 {
		t.Errorf("Connectors count = %d, want 0 for empty source", len(v2Config.Connectors.Connectors))
	}
}

func TestMigrate_PreservesRecoverSettings(t *testing.T) {
	v1Config := ConfigV1{
		Name: "With Recover",
		Recover: RecoverSettings{
			InputJSON:   "/path/to/input.json",
			OutputDir:   "/recover/output",
			Concurrency: 8,
			Delay:       1.5,
			MaxRetry:    5,
			StartFrom:   100,
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if v2Config.Recover.Concurrency != 8 {
		t.Errorf("Recover.Concurrency = %d, want 8", v2Config.Recover.Concurrency)
	}
	if v2Config.Recover.MaxRetry != 5 {
		t.Errorf("Recover.MaxRetry = %d, want 5", v2Config.Recover.MaxRetry)
	}
	if v2Config.Recover.Delay != 1.5 {
		t.Errorf("Recover.Delay = %f, want 1.5", v2Config.Recover.Delay)
	}
}

func TestMigrate_WindowsPathInName(t *testing.T) {
	v1Config := ConfigV1{
		Name: "Windows Path",
		Source: SourceV1{
			Type:         "folder",
			OriginalPath: "C:\\Users\\Test\\Data\\input",
		},
	}

	v1Data, err := json.Marshal(v1Config)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	v2Data, err := Migrate(v1Data, Version1, Version2)
	if err != nil {
		t.Fatalf("Migrate() error: %v", err)
	}

	var v2Config ConfigV2
	if err := json.Unmarshal(v2Data, &v2Config); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(v2Config.Connectors.Connectors) < 1 {
		t.Fatal("Expected at least one connector")
	}

	// Name should be extracted correctly (just "input")
	sourceConn := v2Config.Connectors.Connectors[0]
	if sourceConn.Name != "input" {
		t.Errorf("Name = %q, want 'input'", sourceConn.Name)
	}
}
