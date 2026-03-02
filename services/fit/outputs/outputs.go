// Package outputs provides output modules for Fit data.
package outputs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/fit/metadata"
)

// ── JSON Output ─────────────────────────────────────────────────────────────

// JSONOutput exports fitness data to structured JSON files.
type JSONOutput struct {
	destDir string
	pretty  bool
}

// NewJSON creates a new JSON output.
func NewJSON() *JSONOutput {
	return &JSONOutput{pretty: true}
}

func (o *JSONOutput) ID() string          { return "fit-json" }
func (o *JSONOutput) Name() string        { return "Fit JSON Export" }
func (o *JSONOutput) Description() string { return "Export fitness data to structured JSON" }

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"fit_library"}
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
	if item.Type != "fit_library" {
		return nil
	}

	libraryData, ok := item.Metadata["fit_library"]
	if !ok {
		return fmt.Errorf("missing fit_library in metadata")
	}

	library, ok := libraryData.(*metadata.FitLibrary)
	if !ok {
		return fmt.Errorf("invalid fit_library type in metadata")
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath+".json")
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

// CSVOutput exports fitness data to flat CSV files.
type CSVOutput struct {
	destDir string
}

// NewCSV creates a new CSV output.
func NewCSV() *CSVOutput {
	return &CSVOutput{}
}

func (o *CSVOutput) ID() string          { return "fit-csv" }
func (o *CSVOutput) Name() string        { return "Fit CSV Export" }
func (o *CSVOutput) Description() string { return "Export fitness data to flat CSV files" }

func (o *CSVOutput) SupportedItemTypes() []string {
	return []string{"fit_library"}
}

func (o *CSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for CSV files",
			Type:        "string",
			Required:    true,
		},
	}
}

func (o *CSVOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
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

func (o *CSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// CSV export is batch-only
	return nil
}

func (o *CSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	// Create two CSV files: daily_aggregations.csv and activities.csv

	var allLibraries []*metadata.FitLibrary
	for _, item := range items {
		if item.Type != "fit_library" {
			continue
		}

		libraryData, ok := item.Metadata["fit_library"]
		if !ok {
			continue
		}

		library, ok := libraryData.(*metadata.FitLibrary)
		if !ok {
			continue
		}

		allLibraries = append(allLibraries, library)
	}

	if len(allLibraries) == 0 {
		return nil
	}

	// Write daily aggregations CSV
	if err := o.writeDailyAggregationsCSV(allLibraries); err != nil {
		return err
	}

	// Write activities CSV
	if err := o.writeActivitiesCSV(allLibraries); err != nil {
		return err
	}

	if progress != nil {
		progress(len(items), len(items))
	}

	return nil
}

func (o *CSVOutput) writeDailyAggregationsCSV(libraries []*metadata.FitLibrary) error {
	destPath := filepath.Join(o.destDir, "daily_aggregations.csv")

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"date",
		"move_minutes",
		"calories_kcal",
		"distance_m",
		"heart_points",
		"heart_minutes",
		"steps",
		"avg_heart_rate_bpm",
		"cycling_distance_m",
		"walking_distance_m",
		"running_distance_m",
		"swimming_distance_m",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	// Write data rows
	for _, library := range libraries {
		for _, agg := range library.DailyAggregations {
			row := []string{
				agg.Date.Format("2006-01-02"),
				fmt.Sprintf("%d", agg.MoveMinutes),
				fmt.Sprintf("%.2f", agg.CaloriesExpended),
				fmt.Sprintf("%.2f", agg.DistanceMeters),
				fmt.Sprintf("%d", agg.HeartPoints),
				fmt.Sprintf("%d", agg.HeartMinutes),
				fmt.Sprintf("%d", agg.Steps),
				fmt.Sprintf("%d", agg.AvgHeartRate),
				fmt.Sprintf("%.2f", agg.CyclingDistance),
				fmt.Sprintf("%.2f", agg.WalkingDistance),
				fmt.Sprintf("%.2f", agg.RunningDistance),
				fmt.Sprintf("%.2f", agg.SwimmingDistance),
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
		}
	}

	return nil
}

func (o *CSVOutput) writeActivitiesCSV(libraries []*metadata.FitLibrary) error {
	destPath := filepath.Join(o.destDir, "activities.csv")

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create CSV: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"start_time",
		"end_time",
		"duration_seconds",
		"activity",
		"fitness_activity",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	// Write data rows
	for _, library := range libraries {
		for _, activity := range library.Activities {
			duration := activity.EndTime.Sub(activity.StartTime)
			row := []string{
				activity.StartTime.Format("2006-01-02 15:04:05"),
				activity.EndTime.Format("2006-01-02 15:04:05"),
				fmt.Sprintf("%.0f", duration.Seconds()),
				activity.Activity,
				activity.FitnessActivity,
			}
			if err := writer.Write(row); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
		}
	}

	return nil
}

func (o *CSVOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Registration ────────────────────────────────────────────────────────────

func init() {
	// Register outputs on import
	core.RegisterOutput(NewJSON())
	core.RegisterOutput(NewCSV())
}
