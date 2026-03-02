// Package metadata provides Google Fit activity and fitness data parsing.
package metadata

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	conncore "github.com/fulgidus/revoco/connectors"
)

// DailyAggregation represents aggregated daily fitness data from Google Fit.
type DailyAggregation struct {
	Date             time.Time `json:"date"`
	MoveMinutes      int       `json:"move_minutes,omitempty"`
	CaloriesExpended float64   `json:"calories_expended,omitempty"`
	DistanceMeters   float64   `json:"distance_meters,omitempty"`
	HeartPoints      int       `json:"heart_points,omitempty"`
	HeartMinutes     int       `json:"heart_minutes,omitempty"`
	Steps            int       `json:"steps,omitempty"`
	AvgHeartRate     int       `json:"avg_heart_rate,omitempty"`
	LowLatitude      float64   `json:"low_latitude,omitempty"`
	LowLongitude     float64   `json:"low_longitude,omitempty"`
	HighLatitude     float64   `json:"high_latitude,omitempty"`
	HighLongitude    float64   `json:"high_longitude,omitempty"`
	AvgSpeedMPS      float64   `json:"avg_speed_mps,omitempty"`
	MaxSpeedMPS      float64   `json:"max_speed_mps,omitempty"`
	MinSpeedMPS      float64   `json:"min_speed_mps,omitempty"`
	AvgWeightKG      float64   `json:"avg_weight_kg,omitempty"`
	MaxWeightKG      float64   `json:"max_weight_kg,omitempty"`
	MinWeightKG      float64   `json:"min_weight_kg,omitempty"`
	AvgBodyFat       float64   `json:"avg_body_fat,omitempty"`
	MaxBodyFat       float64   `json:"max_body_fat,omitempty"`
	MinBodyFat       float64   `json:"min_body_fat,omitempty"`
	CyclingDistance  float64   `json:"cycling_distance,omitempty"`
	WalkingDistance  float64   `json:"walking_distance,omitempty"`
	RunningDistance  float64   `json:"running_distance,omitempty"`
	SwimmingDistance float64   `json:"swimming_distance,omitempty"`
}

// ActivitySession represents a single fitness activity session.
type ActivitySession struct {
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	Activity        string    `json:"activity"`
	FitnessActivity string    `json:"fitness_activity"`
}

// FitLibrary represents the complete collection of parsed Fit data.
type FitLibrary struct {
	DailyAggregations []DailyAggregation `json:"daily_aggregations"`
	Activities        []ActivitySession  `json:"activities"`
	Stats             map[string]int     `json:"stats"`
}

// rawActivityJSON represents the structure of activity JSON files from Google Fit Takeout.
type rawActivityJSON struct {
	StartTime       string `json:"startTime"`
	EndTime         string `json:"endTime"`
	Activity        string `json:"activity,omitempty"`
	FitnessActivity string `json:"fitnessActivity,omitempty"`
}

// ParseDailyAggregationCSV parses daily aggregation CSV files from Google Fit.
// CRITICAL: Handles variable columns gracefully - not all users have all metrics.
// Uses header row to map column names to indices.
func ParseDailyAggregationCSV(reader io.Reader) ([]DailyAggregation, error) {
	csvReader := csv.NewReader(reader)

	// Read header row
	header, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	// Build column index map
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[col] = i
	}

	// Verify required column exists
	dateIdx, hasDate := colMap["Date"]
	if !hasDate {
		return nil, fmt.Errorf("missing required 'Date' column")
	}

	var aggregations []DailyAggregation

	// Read data rows
	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row: %w", err)
		}

		// Parse date (required)
		dateStr := row[dateIdx]
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			// Try alternative format
			date, err = time.Parse("01/02/2006", dateStr)
			if err != nil {
				continue // Skip rows with invalid dates
			}
		}

		agg := DailyAggregation{Date: date}

		// Parse optional columns
		agg.MoveMinutes = parseCSVInt(row, colMap, "Move Minutes count")
		agg.CaloriesExpended = parseCSVFloat(row, colMap, "Calories (kcal)")
		agg.DistanceMeters = parseCSVFloat(row, colMap, "Distance (m)")
		agg.HeartPoints = parseCSVInt(row, colMap, "Heart Points")
		agg.HeartMinutes = parseCSVInt(row, colMap, "Heart Minutes")
		agg.Steps = parseCSVInt(row, colMap, "Step count")
		agg.AvgHeartRate = parseCSVInt(row, colMap, "Average heart rate (bpm)")
		agg.LowLatitude = parseCSVFloat(row, colMap, "Low latitude (deg)")
		agg.LowLongitude = parseCSVFloat(row, colMap, "Low longitude (deg)")
		agg.HighLatitude = parseCSVFloat(row, colMap, "High latitude (deg)")
		agg.HighLongitude = parseCSVFloat(row, colMap, "High longitude (deg)")
		agg.AvgSpeedMPS = parseCSVFloat(row, colMap, "Average speed (m/s)")
		agg.MaxSpeedMPS = parseCSVFloat(row, colMap, "Max speed (m/s)")
		agg.MinSpeedMPS = parseCSVFloat(row, colMap, "Min speed (m/s)")
		agg.AvgWeightKG = parseCSVFloat(row, colMap, "Average weight (kg)")
		agg.MaxWeightKG = parseCSVFloat(row, colMap, "Max weight (kg)")
		agg.MinWeightKG = parseCSVFloat(row, colMap, "Min weight (kg)")
		agg.AvgBodyFat = parseCSVFloat(row, colMap, "Average body fat percentage")
		agg.MaxBodyFat = parseCSVFloat(row, colMap, "Max body fat percentage")
		agg.MinBodyFat = parseCSVFloat(row, colMap, "Min body fat percentage")
		agg.CyclingDistance = parseCSVFloat(row, colMap, "Cycling distance (m)")
		agg.WalkingDistance = parseCSVFloat(row, colMap, "Walking distance (m)")
		agg.RunningDistance = parseCSVFloat(row, colMap, "Running distance (m)")
		agg.SwimmingDistance = parseCSVFloat(row, colMap, "Swimming distance (m)")

		aggregations = append(aggregations, agg)
	}

	return aggregations, nil
}

// ParseActivityJSON parses activity session JSON files from Google Fit.
func ParseActivityJSON(reader io.Reader) (*ActivitySession, error) {
	var raw rawActivityJSON
	if err := json.NewDecoder(reader).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode activity JSON: %w", err)
	}

	// Parse timestamps
	startTime, err := time.Parse(time.RFC3339, raw.StartTime)
	if err != nil {
		return nil, fmt.Errorf("parse start time: %w", err)
	}

	endTime, err := time.Parse(time.RFC3339, raw.EndTime)
	if err != nil {
		return nil, fmt.Errorf("parse end time: %w", err)
	}

	activity := &ActivitySession{
		StartTime:       startTime,
		EndTime:         endTime,
		Activity:        raw.Activity,
		FitnessActivity: raw.FitnessActivity,
	}

	return activity, nil
}

// parseCSVInt extracts an integer from a CSV row by column name.
// Returns 0 if column doesn't exist or value is empty/invalid.
func parseCSVInt(row []string, colMap map[string]int, colName string) int {
	idx, ok := colMap[colName]
	if !ok || idx >= len(row) {
		return 0
	}
	val := strings.TrimSpace(row[idx])
	if val == "" {
		return 0
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return i
}

// parseCSVFloat extracts a float from a CSV row by column name.
// Returns 0.0 if column doesn't exist or value is empty/invalid.
func parseCSVFloat(row []string, colMap map[string]int, colName string) float64 {
	idx, ok := colMap[colName]
	if !ok || idx >= len(row) {
		return 0.0
	}
	val := strings.TrimSpace(row[idx])
	if val == "" {
		return 0.0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0.0
	}
	return f
}

// Implement conncore.Metadata interface

// GetTitle returns the title for display.
func (f *FitLibrary) GetTitle() string {
	return "Google Fit Data"
}

// GetDescription returns a description of the data.
func (f *FitLibrary) GetDescription() string {
	totalDays := len(f.DailyAggregations)
	totalActivities := len(f.Activities)
	return fmt.Sprintf("%d daily aggregations, %d activity sessions", totalDays, totalActivities)
}

// GetCreatedDate returns the earliest date in the data.
func (f *FitLibrary) GetCreatedDate() time.Time {
	if len(f.DailyAggregations) == 0 {
		return time.Time{}
	}
	earliest := f.DailyAggregations[0].Date
	for _, agg := range f.DailyAggregations[1:] {
		if agg.Date.Before(earliest) {
			earliest = agg.Date
		}
	}
	// Check activities too
	for _, act := range f.Activities {
		if act.StartTime.Before(earliest) {
			earliest = act.StartTime
		}
	}
	return earliest
}

// GetModifiedDate returns the latest date in the data.
func (f *FitLibrary) GetModifiedDate() time.Time {
	if len(f.DailyAggregations) == 0 {
		return time.Time{}
	}
	latest := f.DailyAggregations[0].Date
	for _, agg := range f.DailyAggregations[1:] {
		if agg.Date.After(latest) {
			latest = agg.Date
		}
	}
	// Check activities too
	for _, act := range f.Activities {
		if act.EndTime.After(latest) {
			latest = act.EndTime
		}
	}
	return latest
}

// GetSize returns the total number of records.
func (f *FitLibrary) GetSize() int64 {
	return int64(len(f.DailyAggregations) + len(f.Activities))
}

// GetDataType returns the data type.
func (f *FitLibrary) GetDataType() string {
	return string(conncore.DataTypeFitnessActivity)
}

// GetMediaType returns the media type.
func (f *FitLibrary) GetMediaType() string {
	return "application/json"
}

// GetMetadata returns all metadata as a map.
func (f *FitLibrary) GetMetadata() map[string]any {
	return map[string]any{
		"daily_aggregations": len(f.DailyAggregations),
		"activity_sessions":  len(f.Activities),
		"date_range_start":   f.GetCreatedDate().Format(time.RFC3339),
		"date_range_end":     f.GetModifiedDate().Format(time.RFC3339),
		"stats":              f.Stats,
	}
}

// SetMetadata updates metadata (no-op for immutable fields).
func (f *FitLibrary) SetMetadata(key string, value any) error {
	return fmt.Errorf("metadata is read-only")
}

// GetTags returns tags/labels (not applicable for fitness data).
func (f *FitLibrary) GetTags() []string {
	return []string{}
}

// SetTags sets tags/labels (not supported).
func (f *FitLibrary) SetTags(tags []string) error {
	return fmt.Errorf("tags not supported for fitness data")
}
