package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// Parser Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestParsePipeline(t *testing.T) {
	yaml := `
name: test-pipeline
description: A test pipeline
jobs:
  fetch:
    steps:
      - uses: google-photos
        with:
          album: "Camera Roll"
  process:
    needs: [fetch]
    steps:
      - uses: heic-to-jpg
        selector:
          extensions: [".heic"]
  export:
    needs: [process]
    steps:
      - uses: local-folder
        with:
          path: ~/Pictures/Backup
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse pipeline: %v", err)
	}

	if pipeline.Name != "test-pipeline" {
		t.Errorf("Expected name 'test-pipeline', got '%s'", pipeline.Name)
	}

	if len(pipeline.Jobs) != 3 {
		t.Errorf("Expected 3 jobs, got %d", len(pipeline.Jobs))
	}

	// Check job dependencies
	processJob := pipeline.Jobs["process"]
	if processJob == nil {
		t.Fatal("Expected 'process' job")
	}
	if len(processJob.Needs) != 1 || processJob.Needs[0] != "fetch" {
		t.Errorf("Expected process to need fetch, got %v", processJob.Needs)
	}
}

func TestParsePipelineWithTrigger(t *testing.T) {
	yaml := `
name: scheduled-backup
on:
  schedule: "0 2 * * *"
jobs:
  backup:
    steps:
      - uses: local-folder
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse pipeline: %v", err)
	}

	if pipeline.On == nil {
		t.Fatal("Expected trigger")
	}

	if pipeline.On.Schedule != "0 2 * * *" {
		t.Errorf("Expected schedule '0 2 * * *', got '%s'", pipeline.On.Schedule)
	}
}

func TestParseInvalidPipeline(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		errMsg string
	}{
		{
			name:   "missing name",
			yaml:   `jobs: {fetch: {steps: [{uses: test}]}}`,
			errMsg: "name",
		},
		{
			name:   "no jobs",
			yaml:   `name: test`,
			errMsg: "jobs",
		},
		{
			name: "empty job",
			yaml: `name: test
jobs:
  empty: {}`,
			errMsg: "step",
		},
		{
			name: "missing uses",
			yaml: `name: test
jobs:
  job1:
    steps:
      - name: step1`,
			errMsg: "uses",
		},
		{
			name: "invalid dependency",
			yaml: `name: test
jobs:
  job1:
    needs: [nonexistent]
    steps:
      - uses: test`,
			errMsg: "nonexistent",
		},
		{
			name: "self dependency",
			yaml: `name: test
jobs:
  job1:
    needs: [job1]
    steps:
      - uses: test`,
			errMsg: "itself",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.errMsg, err.Error())
			}
		})
	}
}

func TestParseCyclicDependency(t *testing.T) {
	yaml := `
name: cyclic
jobs:
  a:
    needs: [c]
    steps:
      - uses: test
  b:
    needs: [a]
    steps:
      - uses: test
  c:
    needs: [b]
    steps:
      - uses: test
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Error("Expected circular dependency error")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("Expected circular dependency error, got: %v", err)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Duration Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestDurationParsing(t *testing.T) {
	yaml := `
name: timeout-test
jobs:
  job1:
    timeout: 30m
    steps:
      - uses: test
        timeout: 5m
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	job := pipeline.Jobs["job1"]
	if job.Timeout.Duration() != 30*time.Minute {
		t.Errorf("Expected job timeout 30m, got %v", job.Timeout.Duration())
	}

	if job.Steps[0].Timeout.Duration() != 5*time.Minute {
		t.Errorf("Expected step timeout 5m, got %v", job.Steps[0].Timeout.Duration())
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Job Order Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestJobOrder(t *testing.T) {
	yaml := `
name: order-test
jobs:
  c:
    needs: [a, b]
    steps:
      - uses: test
  a:
    steps:
      - uses: test
  b:
    needs: [a]
    steps:
      - uses: test
  d:
    needs: [c]
    steps:
      - uses: test
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	order, err := pipeline.JobOrder()
	if err != nil {
		t.Fatalf("Failed to compute order: %v", err)
	}

	// Verify order respects dependencies
	positions := make(map[string]int)
	for i, id := range order {
		positions[id] = i
	}

	// a must come before b
	if positions["a"] >= positions["b"] {
		t.Error("a should come before b")
	}

	// a and b must come before c
	if positions["a"] >= positions["c"] || positions["b"] >= positions["c"] {
		t.Error("a and b should come before c")
	}

	// c must come before d
	if positions["c"] >= positions["d"] {
		t.Error("c should come before d")
	}
}

func TestRootJobs(t *testing.T) {
	yaml := `
name: root-test
jobs:
  root1:
    steps:
      - uses: test
  root2:
    steps:
      - uses: test
  child:
    needs: [root1, root2]
    steps:
      - uses: test
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	roots := pipeline.RootJobs()
	if len(roots) != 2 {
		t.Errorf("Expected 2 root jobs, got %d", len(roots))
	}

	rootSet := make(map[string]bool)
	for _, r := range roots {
		rootSet[r] = true
	}

	if !rootSet["root1"] || !rootSet["root2"] {
		t.Errorf("Expected root1 and root2, got %v", roots)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// DAG Scheduler Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestDAGScheduler(t *testing.T) {
	yaml := `
name: dag-test
jobs:
  a:
    steps:
      - uses: test
  b:
    needs: [a]
    steps:
      - uses: test
  c:
    needs: [a]
    steps:
      - uses: test
  d:
    needs: [b, c]
    steps:
      - uses: test
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	run := NewPipelineRun("test-run", pipeline)
	scheduler := NewDAGScheduler(pipeline, run)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scheduler.Start(ctx)
	defer scheduler.Stop()

	// First job should be 'a'
	firstJob := <-scheduler.Ready()
	if firstJob != "a" {
		t.Errorf("Expected first job 'a', got '%s'", firstJob)
	}

	// Complete 'a'
	scheduler.JobCompleted("a", true, nil)

	// Now b and c should be ready
	readyJobs := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case job := <-scheduler.Ready():
			readyJobs[job] = true
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for ready jobs")
		}
	}

	if !readyJobs["b"] || !readyJobs["c"] {
		t.Errorf("Expected b and c to be ready, got %v", readyJobs)
	}

	// Complete b and c
	scheduler.JobCompleted("b", true, nil)
	scheduler.JobCompleted("c", true, nil)

	// Now d should be ready
	select {
	case job := <-scheduler.Ready():
		if job != "d" {
			t.Errorf("Expected job 'd', got '%s'", job)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for job d")
	}
}

func TestDAGSchedulerFailure(t *testing.T) {
	yaml := `
name: failure-test
jobs:
  a:
    steps:
      - uses: test
  b:
    needs: [a]
    steps:
      - uses: test
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	run := NewPipelineRun("test-run", pipeline)
	scheduler := NewDAGScheduler(pipeline, run)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scheduler.Start(ctx)
	defer scheduler.Stop()

	// Get and fail 'a'
	<-scheduler.Ready()
	scheduler.JobCompleted("a", false, nil)

	// Give scheduler time to process the failure
	time.Sleep(50 * time.Millisecond)

	// 'b' should be skipped (channel should be closed or empty)
	select {
	case job, ok := <-scheduler.Ready():
		if ok && job != "" {
			t.Errorf("Did not expect any job, got '%s'", job)
		}
		// Channel closed or empty string is expected
	case <-time.After(100 * time.Millisecond):
		// Expected - no more jobs should be ready
	}

	if !scheduler.HasFailures() {
		t.Error("Expected scheduler to report failures")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Dependency Graph Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestDependencyGraph(t *testing.T) {
	yaml := `
name: graph-test
jobs:
  a:
    steps:
      - uses: test
  b:
    needs: [a]
    steps:
      - uses: test
  c:
    needs: [a]
    steps:
      - uses: test
  d:
    needs: [b, c]
    steps:
      - uses: test
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	graph, err := BuildDependencyGraph(pipeline)
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	// Check levels
	levels := graph.Levels()
	if len(levels) != 3 {
		t.Errorf("Expected 3 levels, got %d", len(levels))
	}

	// Level 0: a
	// Level 1: b, c
	// Level 2: d
	if len(levels[0]) != 1 || levels[0][0] != "a" {
		t.Errorf("Expected level 0 = [a], got %v", levels[0])
	}
	if len(levels[1]) != 2 {
		t.Errorf("Expected level 1 to have 2 jobs, got %d", len(levels[1]))
	}
	if len(levels[2]) != 1 || levels[2][0] != "d" {
		t.Errorf("Expected level 2 = [d], got %v", levels[2])
	}
}

func TestCriticalPath(t *testing.T) {
	yaml := `
name: critical-path-test
jobs:
  a:
    steps:
      - uses: test
  b:
    needs: [a]
    steps:
      - uses: test
  c:
    needs: [b]
    steps:
      - uses: test
  side:
    needs: [a]
    steps:
      - uses: test
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	graph, err := BuildDependencyGraph(pipeline)
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	path := graph.CriticalPath()
	if len(path) != 3 {
		t.Errorf("Expected critical path length 3, got %d: %v", len(path), path)
	}

	// Critical path should be a -> b -> c
	expected := []string{"a", "b", "c"}
	for i, job := range expected {
		if i >= len(path) || path[i] != job {
			t.Errorf("Expected critical path %v, got %v", expected, path)
			break
		}
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Pipeline Run Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestPipelineRun(t *testing.T) {
	yaml := `
name: run-test
jobs:
  a:
    steps:
      - uses: test
  b:
    steps:
      - uses: test
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	run := NewPipelineRun("test-run", pipeline)

	if run.State != PipelineStatePending {
		t.Errorf("Expected pending state, got %s", run.State)
	}

	if run.TotalJobs() != 2 {
		t.Errorf("Expected 2 jobs, got %d", run.TotalJobs())
	}

	if run.CompletedJobs() != 0 {
		t.Errorf("Expected 0 completed jobs, got %d", run.CompletedJobs())
	}

	if run.Progress() != 0.0 {
		t.Errorf("Expected progress 0.0, got %f", run.Progress())
	}

	// Simulate completing jobs
	run.Jobs["a"].State = JobStateCompleted
	if run.CompletedJobs() != 1 {
		t.Errorf("Expected 1 completed job, got %d", run.CompletedJobs())
	}

	run.Jobs["b"].State = JobStateCompleted
	if run.Progress() != 1.0 {
		t.Errorf("Expected progress 1.0, got %f", run.Progress())
	}
}

func TestPipelineRunCancel(t *testing.T) {
	yaml := `
name: cancel-test
jobs:
  a:
    steps:
      - uses: test
`
	pipeline, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	run := NewPipelineRun("test-run", pipeline)
	run.Cancel()

	if run.State != PipelineStateCancelled {
		t.Errorf("Expected cancelled state, got %s", run.State)
	}

	// Context should be cancelled
	select {
	case <-run.Context().Done():
		// Expected
	default:
		t.Error("Expected context to be cancelled")
	}
}
