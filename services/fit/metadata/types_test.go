package metadata

import (
	"strings"
	"testing"
	"time"
)

func TestParseDailyAggregationCSV_Valid(t *testing.T) {
	csvData := `Date,Move Minutes count,Calories (kcal),Distance (m),Heart Points,Heart Minutes,Step count,Average heart rate (bpm)
2024-01-15,60,2500.5,8500.2,30,45,10500,72
2024-01-16,45,2100.0,6000.0,25,40,8000,70`

	reader := strings.NewReader(csvData)
	aggs, err := ParseDailyAggregationCSV(reader)

	if err != nil {
		t.Fatalf("ParseDailyAggregationCSV failed: %v", err)
	}

	if len(aggs) != 2 {
		t.Errorf("Expected 2 aggregations, got %d", len(aggs))
	}

	// Check first record
	first := aggs[0]
	expectedDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if !first.Date.Equal(expectedDate) {
		t.Errorf("Expected date %v, got %v", expectedDate, first.Date)
	}
	if first.MoveMinutes != 60 {
		t.Errorf("Expected MoveMinutes 60, got %d", first.MoveMinutes)
	}
	if first.CaloriesExpended != 2500.5 {
		t.Errorf("Expected CaloriesExpended 2500.5, got %f", first.CaloriesExpended)
	}
	if first.Steps != 10500 {
		t.Errorf("Expected Steps 10500, got %d", first.Steps)
	}
	if first.AvgHeartRate != 72 {
		t.Errorf("Expected AvgHeartRate 72, got %d", first.AvgHeartRate)
	}
}

func TestParseDailyAggregationCSV_MissingColumns(t *testing.T) {
	// CSV without heart rate columns (common for users without heart rate monitors)
	csvData := `Date,Move Minutes count,Calories (kcal),Distance (m),Step count
2024-01-15,60,2500.5,8500.2,10500
2024-01-16,45,2100.0,6000.0,8000`

	reader := strings.NewReader(csvData)
	aggs, err := ParseDailyAggregationCSV(reader)

	if err != nil {
		t.Fatalf("ParseDailyAggregationCSV failed: %v", err)
	}

	if len(aggs) != 2 {
		t.Errorf("Expected 2 aggregations, got %d", len(aggs))
	}

	// Check that missing columns default to zero
	first := aggs[0]
	if first.AvgHeartRate != 0 {
		t.Errorf("Expected AvgHeartRate 0 (missing column), got %d", first.AvgHeartRate)
	}
	if first.HeartPoints != 0 {
		t.Errorf("Expected HeartPoints 0 (missing column), got %d", first.HeartPoints)
	}
	// Existing columns should parse correctly
	if first.Steps != 10500 {
		t.Errorf("Expected Steps 10500, got %d", first.Steps)
	}
}

func TestParseDailyAggregationCSV_Empty(t *testing.T) {
	csvData := `Date,Move Minutes count,Calories (kcal)
`

	reader := strings.NewReader(csvData)
	aggs, err := ParseDailyAggregationCSV(reader)

	if err != nil {
		t.Fatalf("ParseDailyAggregationCSV failed: %v", err)
	}

	if len(aggs) != 0 {
		t.Errorf("Expected 0 aggregations, got %d", len(aggs))
	}
}

func TestParseDailyAggregationCSV_MissingDateColumn(t *testing.T) {
	csvData := `Move Minutes count,Calories (kcal)
60,2500.5`

	reader := strings.NewReader(csvData)
	_, err := ParseDailyAggregationCSV(reader)

	if err == nil {
		t.Error("Expected error for missing Date column, got nil")
	}
}

func TestParseDailyAggregationCSV_InvalidDate(t *testing.T) {
	csvData := `Date,Move Minutes count,Step count
invalid-date,60,10500
2024-01-16,45,8000`

	reader := strings.NewReader(csvData)
	aggs, err := ParseDailyAggregationCSV(reader)

	if err != nil {
		t.Fatalf("ParseDailyAggregationCSV failed: %v", err)
	}

	// Should skip invalid row and parse valid row
	if len(aggs) != 1 {
		t.Errorf("Expected 1 aggregation (skipped invalid), got %d", len(aggs))
	}
}

func TestParseDailyAggregationCSV_EmptyValues(t *testing.T) {
	csvData := `Date,Move Minutes count,Calories (kcal),Step count
2024-01-15,,2500.5,
2024-01-16,45,,8000`

	reader := strings.NewReader(csvData)
	aggs, err := ParseDailyAggregationCSV(reader)

	if err != nil {
		t.Fatalf("ParseDailyAggregationCSV failed: %v", err)
	}

	if len(aggs) != 2 {
		t.Errorf("Expected 2 aggregations, got %d", len(aggs))
	}

	// Empty values should default to zero
	first := aggs[0]
	if first.MoveMinutes != 0 {
		t.Errorf("Expected MoveMinutes 0 (empty), got %d", first.MoveMinutes)
	}
	if first.Steps != 0 {
		t.Errorf("Expected Steps 0 (empty), got %d", first.Steps)
	}
	if first.CaloriesExpended != 2500.5 {
		t.Errorf("Expected CaloriesExpended 2500.5, got %f", first.CaloriesExpended)
	}
}

func TestParseActivityJSON_Valid(t *testing.T) {
	jsonData := `{
		"startTime": "2024-01-15T10:30:00.000Z",
		"endTime": "2024-01-15T11:00:00.000Z",
		"activity": "WALKING",
		"fitnessActivity": "walking"
	}`

	reader := strings.NewReader(jsonData)
	activity, err := ParseActivityJSON(reader)

	if err != nil {
		t.Fatalf("ParseActivityJSON failed: %v", err)
	}

	expectedStart := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	expectedEnd := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	if !activity.StartTime.Equal(expectedStart) {
		t.Errorf("Expected StartTime %v, got %v", expectedStart, activity.StartTime)
	}
	if !activity.EndTime.Equal(expectedEnd) {
		t.Errorf("Expected EndTime %v, got %v", expectedEnd, activity.EndTime)
	}
	if activity.Activity != "WALKING" {
		t.Errorf("Expected Activity WALKING, got %s", activity.Activity)
	}
	if activity.FitnessActivity != "walking" {
		t.Errorf("Expected FitnessActivity walking, got %s", activity.FitnessActivity)
	}
}

func TestParseActivityJSON_MissingFields(t *testing.T) {
	jsonData := `{
		"startTime": "2024-01-15T10:30:00.000Z",
		"endTime": "2024-01-15T11:00:00.000Z"
	}`

	reader := strings.NewReader(jsonData)
	activity, err := ParseActivityJSON(reader)

	if err != nil {
		t.Fatalf("ParseActivityJSON failed: %v", err)
	}

	if activity.Activity != "" {
		t.Errorf("Expected empty Activity, got %s", activity.Activity)
	}
	if activity.FitnessActivity != "" {
		t.Errorf("Expected empty FitnessActivity, got %s", activity.FitnessActivity)
	}
}

func TestParseActivityJSON_InvalidTimestamp(t *testing.T) {
	jsonData := `{
		"startTime": "invalid-timestamp",
		"endTime": "2024-01-15T11:00:00.000Z"
	}`

	reader := strings.NewReader(jsonData)
	_, err := ParseActivityJSON(reader)

	if err == nil {
		t.Error("Expected error for invalid timestamp, got nil")
	}
}

func TestParseActivityJSON_Malformed(t *testing.T) {
	jsonData := `{invalid json}`

	reader := strings.NewReader(jsonData)
	_, err := ParseActivityJSON(reader)

	if err == nil {
		t.Error("Expected error for malformed JSON, got nil")
	}
}

func TestFitLibrary_GetTitle(t *testing.T) {
	lib := &FitLibrary{}
	if lib.GetTitle() != "Google Fit Data" {
		t.Errorf("Expected title 'Google Fit Data', got %s", lib.GetTitle())
	}
}

func TestFitLibrary_GetDescription(t *testing.T) {
	lib := &FitLibrary{
		DailyAggregations: []DailyAggregation{{}, {}},
		Activities:        []ActivitySession{{}, {}, {}},
	}

	desc := lib.GetDescription()
	expected := "2 daily aggregations, 3 activity sessions"
	if desc != expected {
		t.Errorf("Expected description '%s', got '%s'", expected, desc)
	}
}

func TestFitLibrary_GetCreatedDate(t *testing.T) {
	lib := &FitLibrary{
		DailyAggregations: []DailyAggregation{
			{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
			{Date: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)},
		},
		Activities: []ActivitySession{
			{StartTime: time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)},
		},
	}

	created := lib.GetCreatedDate()
	expected := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	if !created.Equal(expected) {
		t.Errorf("Expected created date %v, got %v", expected, created)
	}
}

func TestFitLibrary_GetModifiedDate(t *testing.T) {
	lib := &FitLibrary{
		DailyAggregations: []DailyAggregation{
			{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)},
			{Date: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)},
		},
		Activities: []ActivitySession{
			{EndTime: time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)},
		},
	}

	modified := lib.GetModifiedDate()
	expected := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	if !modified.Equal(expected) {
		t.Errorf("Expected modified date %v, got %v", expected, modified)
	}
}

func TestFitLibrary_GetSize(t *testing.T) {
	lib := &FitLibrary{
		DailyAggregations: []DailyAggregation{{}, {}},
		Activities:        []ActivitySession{{}, {}, {}},
	}

	size := lib.GetSize()
	expected := int64(5)
	if size != expected {
		t.Errorf("Expected size %d, got %d", expected, size)
	}
}

func TestFitLibrary_GetDataType(t *testing.T) {
	lib := &FitLibrary{}
	dataType := lib.GetDataType()
	if dataType != "fitness_activity" {
		t.Errorf("Expected data type 'fitness_activity', got %s", dataType)
	}
}

func TestFitLibrary_GetMetadata(t *testing.T) {
	lib := &FitLibrary{
		DailyAggregations: []DailyAggregation{{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)}},
		Activities:        []ActivitySession{{StartTime: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)}},
		Stats:             map[string]int{"total_steps": 50000},
	}

	meta := lib.GetMetadata()

	if meta["daily_aggregations"] != 1 {
		t.Errorf("Expected daily_aggregations 1, got %v", meta["daily_aggregations"])
	}
	if meta["activity_sessions"] != 1 {
		t.Errorf("Expected activity_sessions 1, got %v", meta["activity_sessions"])
	}
}

func TestFitLibrary_SetMetadata_ReadOnly(t *testing.T) {
	lib := &FitLibrary{}
	err := lib.SetMetadata("key", "value")

	if err == nil {
		t.Error("Expected error for read-only metadata, got nil")
	}
}

func TestFitLibrary_SetTags_NotSupported(t *testing.T) {
	lib := &FitLibrary{}
	err := lib.SetTags([]string{"tag1", "tag2"})

	if err == nil {
		t.Error("Expected error for unsupported tags, got nil")
	}
}

func TestFitLibrary_GetTags_Empty(t *testing.T) {
	lib := &FitLibrary{}
	tags := lib.GetTags()

	if len(tags) != 0 {
		t.Errorf("Expected empty tags, got %v", tags)
	}
}
