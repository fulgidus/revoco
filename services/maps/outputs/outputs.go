// Package outputs provides output modules for Maps data.
package outputs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/maps/metadata"
)

// ── GeoJSON Output ──────────────────────────────────────────────────────────

// GeoJSONOutput exports location data to GeoJSON format (RFC 7946).
type GeoJSONOutput struct {
	destDir string
	pretty  bool
}

// NewGeoJSON creates a new GeoJSON output.
func NewGeoJSON() *GeoJSONOutput {
	return &GeoJSONOutput{pretty: true}
}

func (o *GeoJSONOutput) ID() string          { return "maps-geojson" }
func (o *GeoJSONOutput) Name() string        { return "Maps GeoJSON Export" }
func (o *GeoJSONOutput) Description() string { return "Export location data to GeoJSON format" }

func (o *GeoJSONOutput) SupportedItemTypes() []string {
	return []string{"maps_library"}
}

func (o *GeoJSONOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for GeoJSON files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "pretty",
			Name:        "Pretty Print",
			Description: "Format JSON with indentation",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *GeoJSONOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["pretty"].(bool); ok {
		o.pretty = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *GeoJSONOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "maps_library" {
		return nil
	}

	libraryData, ok := item.Metadata["maps_library"]
	if !ok {
		return fmt.Errorf("missing maps_library in metadata")
	}

	library, ok := libraryData.(*metadata.MapsLibrary)
	if !ok {
		return fmt.Errorf("invalid maps_library type in metadata")
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	// Change extension to .geojson
	destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".geojson"

	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Build GeoJSON FeatureCollection
	geojson := map[string]any{
		"type":     "FeatureCollection",
		"features": []map[string]any{},
	}

	features := geojson["features"].([]map[string]any)

	// Add location history points
	for _, record := range library.LocationHistory {
		feature := map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []float64{record.Longitude, record.Latitude}, // GeoJSON: [lon, lat]
			},
			"properties": map[string]any{
				"timestamp": record.Timestamp.Format("2006-01-02T15:04:05Z"),
				"accuracy":  record.Accuracy,
				"source":    record.Source,
				"altitude":  record.Altitude,
			},
		}
		features = append(features, feature)
	}

	// Add saved places
	for _, place := range library.SavedPlaces {
		feature := map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []float64{place.Longitude, place.Latitude},
			},
			"properties": map[string]any{
				"name":            place.Name,
				"address":         place.Address,
				"note":            place.Note,
				"google_maps_url": place.GoogleMapsURL,
				"type":            "saved_place",
			},
		}
		features = append(features, feature)
	}

	// Add timeline visits
	for _, visit := range library.Timeline {
		feature := map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": []float64{visit.Location.Longitude, visit.Location.Latitude},
			},
			"properties": map[string]any{
				"name":       visit.Location.Name,
				"address":    visit.Location.Address,
				"start_time": visit.StartTime.Format("2006-01-02T15:04:05Z"),
				"end_time":   visit.EndTime.Format("2006-01-02T15:04:05Z"),
				"duration":   visit.Duration.String(),
				"confidence": visit.Confidence,
				"type":       "place_visit",
			},
		}
		features = append(features, feature)
	}

	geojson["features"] = features

	// Write GeoJSON
	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(geojson, "", "  ")
	} else {
		data, err = json.Marshal(geojson)
	}
	if err != nil {
		return fmt.Errorf("marshal GeoJSON: %w", err)
	}

	return os.WriteFile(destPath, data, 0o644)
}

func (o *GeoJSONOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *GeoJSONOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── KML Output ──────────────────────────────────────────────────────────────

// KMLOutput exports location data to KML format (OGC KML 2.2).
type KMLOutput struct {
	destDir string
}

// NewKML creates a new KML output.
func NewKML() *KMLOutput {
	return &KMLOutput{}
}

func (o *KMLOutput) ID() string          { return "maps-kml" }
func (o *KMLOutput) Name() string        { return "Maps KML Export" }
func (o *KMLOutput) Description() string { return "Export location data to KML format" }

func (o *KMLOutput) SupportedItemTypes() []string {
	return []string{"maps_library"}
}

func (o *KMLOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for KML files",
			Type:        "string",
			Required:    true,
		},
	}
}

func (o *KMLOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *KMLOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "maps_library" {
		return nil
	}

	libraryData, ok := item.Metadata["maps_library"]
	if !ok {
		return fmt.Errorf("missing maps_library in metadata")
	}

	library, ok := libraryData.(*metadata.MapsLibrary)
	if !ok {
		return fmt.Errorf("invalid maps_library type in metadata")
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	// Change extension to .kml
	destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".kml"

	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Build KML document
	var kml strings.Builder
	kml.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	kml.WriteString("\n")
	kml.WriteString(`<kml xmlns="http://www.opengis.net/kml/2.2">`)
	kml.WriteString("\n")
	kml.WriteString("  <Document>\n")
	kml.WriteString("    <name>Google Maps Location Data</name>\n")

	// Add location history points
	if len(library.LocationHistory) > 0 {
		kml.WriteString("    <Folder>\n")
		kml.WriteString("      <name>Location History</name>\n")
		for _, record := range library.LocationHistory {
			kml.WriteString("      <Placemark>\n")
			kml.WriteString(fmt.Sprintf("        <name>%s</name>\n", record.Timestamp.Format("2006-01-02 15:04:05")))
			kml.WriteString(fmt.Sprintf("        <description>Accuracy: %dm, Source: %s</description>\n", record.Accuracy, record.Source))
			kml.WriteString("        <Point>\n")
			// KML format: lon,lat,alt
			kml.WriteString(fmt.Sprintf("          <coordinates>%.7f,%.7f,%.1f</coordinates>\n", record.Longitude, record.Latitude, record.Altitude))
			kml.WriteString("        </Point>\n")
			kml.WriteString("      </Placemark>\n")
		}
		kml.WriteString("    </Folder>\n")
	}

	// Add saved places
	if len(library.SavedPlaces) > 0 {
		kml.WriteString("    <Folder>\n")
		kml.WriteString("      <name>Saved Places</name>\n")
		for _, place := range library.SavedPlaces {
			kml.WriteString("      <Placemark>\n")
			kml.WriteString(fmt.Sprintf("        <name>%s</name>\n", escapeXML(place.Name)))
			if place.Address != "" || place.Note != "" {
				desc := place.Address
				if place.Note != "" {
					if desc != "" {
						desc += " - "
					}
					desc += place.Note
				}
				kml.WriteString(fmt.Sprintf("        <description>%s</description>\n", escapeXML(desc)))
			}
			kml.WriteString("        <Point>\n")
			kml.WriteString(fmt.Sprintf("          <coordinates>%.7f,%.7f,0</coordinates>\n", place.Longitude, place.Latitude))
			kml.WriteString("        </Point>\n")
			kml.WriteString("      </Placemark>\n")
		}
		kml.WriteString("    </Folder>\n")
	}

	// Add timeline visits
	if len(library.Timeline) > 0 {
		kml.WriteString("    <Folder>\n")
		kml.WriteString("      <name>Timeline Visits</name>\n")
		for _, visit := range library.Timeline {
			kml.WriteString("      <Placemark>\n")
			kml.WriteString(fmt.Sprintf("        <name>%s</name>\n", escapeXML(visit.Location.Name)))
			desc := fmt.Sprintf("%s to %s (%s)", visit.StartTime.Format("2006-01-02 15:04"), visit.EndTime.Format("15:04"), visit.Duration)
			kml.WriteString(fmt.Sprintf("        <description>%s</description>\n", escapeXML(desc)))
			kml.WriteString("        <Point>\n")
			kml.WriteString(fmt.Sprintf("          <coordinates>%.7f,%.7f,0</coordinates>\n", visit.Location.Longitude, visit.Location.Latitude))
			kml.WriteString("        </Point>\n")
			kml.WriteString("      </Placemark>\n")
		}
		kml.WriteString("    </Folder>\n")
	}

	kml.WriteString("  </Document>\n")
	kml.WriteString("</kml>\n")

	return os.WriteFile(destPath, []byte(kml.String()), 0o644)
}

func (o *KMLOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *KMLOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── JSON Output ─────────────────────────────────────────────────────────────

// JSONOutput exports location data to structured JSON files.
type JSONOutput struct {
	destDir string
	pretty  bool
}

// NewJSON creates a new JSON output.
func NewJSON() *JSONOutput {
	return &JSONOutput{pretty: true}
}

func (o *JSONOutput) ID() string          { return "maps-json" }
func (o *JSONOutput) Name() string        { return "Maps JSON Export" }
func (o *JSONOutput) Description() string { return "Export location data to structured JSON" }

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"maps_library"}
}

func (o *JSONOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for JSON files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "pretty",
			Name:        "Pretty Print",
			Description: "Format JSON with indentation",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *JSONOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["pretty"].(bool); ok {
		o.pretty = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *JSONOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "maps_library" {
		return nil
	}

	libraryData, ok := item.Metadata["maps_library"]
	if !ok {
		return fmt.Errorf("missing maps_library in metadata")
	}

	library, ok := libraryData.(*metadata.MapsLibrary)
	if !ok {
		return fmt.Errorf("invalid maps_library type in metadata")
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	os.MkdirAll(filepath.Dir(destPath), 0o755)

	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(library, "", "  ")
	} else {
		data, err = json.Marshal(library)
	}
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	return os.WriteFile(destPath, data, 0o644)
}

func (o *JSONOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *JSONOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── CSV Output ──────────────────────────────────────────────────────────────

// CSVOutput exports location data to flat CSV files.
type CSVOutput struct {
	destPath string
}

// NewCSV creates a new CSV output.
func NewCSV() *CSVOutput {
	return &CSVOutput{}
}

func (o *CSVOutput) ID() string          { return "maps-csv" }
func (o *CSVOutput) Name() string        { return "Maps CSV Export" }
func (o *CSVOutput) Description() string { return "Export location data to flat CSV format" }

func (o *CSVOutput) SupportedItemTypes() []string {
	return []string{"maps_library"}
}

func (o *CSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_path",
			Name:        "Destination File",
			Description: "Output CSV file path (e.g., locations.csv)",
			Type:        "string",
			Required:    true,
		},
	}
}

func (o *CSVOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destPath = cfg.DestDir
	if o.destPath == "" {
		if d, ok := cfg.Settings["dest_path"].(string); ok {
			o.destPath = d
		}
	}
	if o.destPath == "" {
		o.destPath = filepath.Join(cfg.DestDir, "locations.csv")
	}

	return os.MkdirAll(filepath.Dir(o.destPath), 0o755)
}

func (o *CSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// CSV export is batch-only
	return nil
}

func (o *CSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	file, err := os.Create(o.destPath)
	if err != nil {
		return fmt.Errorf("create CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"type",
		"timestamp",
		"latitude",
		"longitude",
		"accuracy",
		"source",
		"name",
		"address",
		"note",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	// Write location records
	totalWritten := 0
	for _, item := range items {
		if item.Type != "maps_library" {
			continue
		}

		libraryData, ok := item.Metadata["maps_library"]
		if !ok {
			continue
		}

		library, ok := libraryData.(*metadata.MapsLibrary)
		if !ok {
			continue
		}

		// Write location history
		for _, record := range library.LocationHistory {
			row := []string{
				"location_record",
				record.Timestamp.Format("2006-01-02 15:04:05"),
				fmt.Sprintf("%.7f", record.Latitude),
				fmt.Sprintf("%.7f", record.Longitude),
				fmt.Sprintf("%d", record.Accuracy),
				record.Source,
				"",
				"",
				"",
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
			totalWritten++
		}

		// Write saved places
		for _, place := range library.SavedPlaces {
			row := []string{
				"saved_place",
				"",
				fmt.Sprintf("%.7f", place.Latitude),
				fmt.Sprintf("%.7f", place.Longitude),
				"",
				"",
				place.Name,
				place.Address,
				place.Note,
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
			totalWritten++
		}

		// Write timeline visits
		for _, visit := range library.Timeline {
			row := []string{
				"place_visit",
				visit.StartTime.Format("2006-01-02 15:04:05"),
				fmt.Sprintf("%.7f", visit.Location.Latitude),
				fmt.Sprintf("%.7f", visit.Location.Longitude),
				"",
				"",
				visit.Location.Name,
				visit.Location.Address,
				visit.Duration.String(),
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
			totalWritten++
		}
	}

	if progress != nil {
		progress(totalWritten, totalWritten)
	}

	return nil
}

func (o *CSVOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Helper functions ────────────────────────────────────────────────────────

// escapeXML escapes special XML characters.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// ── Registration ────────────────────────────────────────────────────────────

func init() {
	// Register outputs on import
	core.RegisterOutput(NewGeoJSON())
	core.RegisterOutput(NewKML())
	core.RegisterOutput(NewJSON())
	core.RegisterOutput(NewCSV())
}
