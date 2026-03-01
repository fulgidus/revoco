// Package pipeline provides the pipeline engine for revoco.
//
// Pipelines define workflows using GitHub Actions-style YAML with jobs
// containing sequential steps. Jobs can run in parallel based on their
// dependencies (needs).
//
// Example pipeline:
//
//	name: backup-photos
//	on:
//	  schedule: "0 2 * * *"
//	jobs:
//	  fetch:
//	    steps:
//	      - uses: google-photos
//	        with:
//	          album: "Camera Roll"
//	  process:
//	    needs: [fetch]
//	    steps:
//	      - uses: heic-to-jpg
//	        selector:
//	          extensions: [".heic"]
//	  export:
//	    needs: [process]
//	    steps:
//	      - uses: local-folder
//	        with:
//	          path: ~/Pictures/Backup
package pipeline

import (
	"context"
	"time"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/plugins"
)

// ══════════════════════════════════════════════════════════════════════════════
// Pipeline Types
// ══════════════════════════════════════════════════════════════════════════════

// Pipeline represents a complete workflow definition.
type Pipeline struct {
	// Identity
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Version     string `yaml:"version,omitempty" json:"version,omitempty"`

	// Triggers
	On *Trigger `yaml:"on,omitempty" json:"on,omitempty"`

	// Environment variables available to all jobs
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`

	// Jobs to execute
	Jobs map[string]*Job `yaml:"jobs" json:"jobs"`

	// Source file path (set during loading)
	SourcePath string `yaml:"-" json:"-"`
}

// Trigger defines when a pipeline should run.
type Trigger struct {
	// Cron schedule (e.g., "0 2 * * *" for 2 AM daily)
	Schedule string `yaml:"schedule,omitempty" json:"schedule,omitempty"`

	// Run on specific events
	Events []string `yaml:"events,omitempty" json:"events,omitempty"`

	// Manual trigger only
	Manual bool `yaml:"manual,omitempty" json:"manual,omitempty"`
}

// Job represents a unit of work in the pipeline.
// Jobs run in parallel unless they have dependencies.
type Job struct {
	// Display name
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Job dependencies (other job IDs that must complete first)
	Needs []string `yaml:"needs,omitempty" json:"needs,omitempty"`

	// Condition to run this job (Lua expression)
	If string `yaml:"if,omitempty" json:"if,omitempty"`

	// Environment variables for this job
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`

	// Steps to execute (in order)
	Steps []*Step `yaml:"steps" json:"steps"`

	// Timeout for the entire job
	Timeout Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// Continue pipeline even if this job fails
	ContinueOnError bool `yaml:"continue-on-error,omitempty" json:"continue-on-error,omitempty"`

	// Runtime state (not serialized)
	ID string `yaml:"-" json:"-"` // Set during parsing
}

// Step represents a single action within a job.
type Step struct {
	// Step identifier (optional, auto-generated if not provided)
	ID string `yaml:"id,omitempty" json:"id,omitempty"`

	// Display name
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Plugin to use (e.g., "google-photos", "heic-to-jpg")
	Uses string `yaml:"uses" json:"uses"`

	// Plugin configuration
	With map[string]any `yaml:"with,omitempty" json:"with,omitempty"`

	// Item selector (for processors and outputs)
	Selector *plugins.Selector `yaml:"selector,omitempty" json:"selector,omitempty"`

	// Condition to run this step (Lua expression)
	If string `yaml:"if,omitempty" json:"if,omitempty"`

	// Timeout for this step
	Timeout Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// Continue job even if this step fails
	ContinueOnError bool `yaml:"continue-on-error,omitempty" json:"continue-on-error,omitempty"`

	// Environment variables for this step
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
}

// Duration is a wrapper for time.Duration that supports YAML parsing.
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler for Duration.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		*d = 0
		return nil
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(duration)
	return nil
}

// MarshalYAML implements yaml.Marshaler for Duration.
func (d Duration) MarshalYAML() (interface{}, error) {
	if d == 0 {
		return "", nil
	}
	return time.Duration(d).String(), nil
}

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// ══════════════════════════════════════════════════════════════════════════════
// Execution State Types
// ══════════════════════════════════════════════════════════════════════════════

// PipelineState represents the execution state of a pipeline.
type PipelineState string

const (
	PipelineStatePending   PipelineState = "pending"
	PipelineStateRunning   PipelineState = "running"
	PipelineStateCompleted PipelineState = "completed"
	PipelineStateFailed    PipelineState = "failed"
	PipelineStateCancelled PipelineState = "cancelled"
)

// JobState represents the execution state of a job.
type JobState string

const (
	JobStatePending   JobState = "pending"
	JobStateQueued    JobState = "queued"
	JobStateRunning   JobState = "running"
	JobStateCompleted JobState = "completed"
	JobStateFailed    JobState = "failed"
	JobStateSkipped   JobState = "skipped"
	JobStateCancelled JobState = "cancelled"
)

// StepState represents the execution state of a step.
type StepState string

const (
	StepStatePending   StepState = "pending"
	StepStateRunning   StepState = "running"
	StepStateCompleted StepState = "completed"
	StepStateFailed    StepState = "failed"
	StepStateSkipped   StepState = "skipped"
)

// ══════════════════════════════════════════════════════════════════════════════
// Execution Context Types
// ══════════════════════════════════════════════════════════════════════════════

// PipelineRun represents a single execution of a pipeline.
type PipelineRun struct {
	// Identity
	ID         string    `json:"id"`
	PipelineID string    `json:"pipeline_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`

	// State
	State   PipelineState `json:"state"`
	Message string        `json:"message,omitempty"`

	// Job runs
	Jobs map[string]*JobRun `json:"jobs"`

	// Data passed between jobs
	Items []*core.DataItem `json:"-"`

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// JobRun represents a single execution of a job.
type JobRun struct {
	// Identity
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	StartTime time.Time `json:"start_time,omitempty"`
	EndTime   time.Time `json:"end_time,omitempty"`

	// State
	State   JobState `json:"state"`
	Message string   `json:"message,omitempty"`

	// Step runs
	Steps []*StepRun `json:"steps"`

	// Items at job start and end
	InputItems  []*core.DataItem `json:"-"`
	OutputItems []*core.DataItem `json:"-"`
}

// StepRun represents a single execution of a step.
type StepRun struct {
	// Identity
	ID        string    `json:"id"`
	StepID    string    `json:"step_id"`
	StepIndex int       `json:"step_index"`
	StartTime time.Time `json:"start_time,omitempty"`
	EndTime   time.Time `json:"end_time,omitempty"`

	// State
	State   StepState `json:"state"`
	Message string    `json:"message,omitempty"`

	// Progress
	ItemsProcessed int `json:"items_processed"`
	ItemsTotal     int `json:"items_total"`

	// Items at step start and end
	InputItems  []*core.DataItem `json:"-"`
	OutputItems []*core.DataItem `json:"-"`
}

// ══════════════════════════════════════════════════════════════════════════════
// Progress Callback Types
// ══════════════════════════════════════════════════════════════════════════════

// PipelineProgress reports pipeline execution progress.
type PipelineProgress struct {
	Run      *PipelineRun
	Job      *JobRun
	Step     *StepRun
	Event    ProgressEvent
	Progress float64 // 0.0 to 1.0
	Message  string
}

// ProgressEvent indicates what kind of progress update occurred.
type ProgressEvent string

const (
	EventPipelineStarted   ProgressEvent = "pipeline_started"
	EventPipelineCompleted ProgressEvent = "pipeline_completed"
	EventPipelineFailed    ProgressEvent = "pipeline_failed"
	EventJobStarted        ProgressEvent = "job_started"
	EventJobCompleted      ProgressEvent = "job_completed"
	EventJobFailed         ProgressEvent = "job_failed"
	EventJobSkipped        ProgressEvent = "job_skipped"
	EventStepStarted       ProgressEvent = "step_started"
	EventStepCompleted     ProgressEvent = "step_completed"
	EventStepFailed        ProgressEvent = "step_failed"
	EventStepProgress      ProgressEvent = "step_progress"
)

// ProgressCallback is called when pipeline execution progress changes.
type ProgressCallback func(progress PipelineProgress)

// ══════════════════════════════════════════════════════════════════════════════
// Executor Interface
// ══════════════════════════════════════════════════════════════════════════════

// Executor runs pipelines.
type Executor interface {
	// Execute runs a pipeline and returns when complete.
	Execute(ctx context.Context, pipeline *Pipeline, progress ProgressCallback) (*PipelineRun, error)

	// Cancel stops a running pipeline.
	Cancel(runID string) error

	// Status returns the current status of a pipeline run.
	Status(runID string) (*PipelineRun, error)
}

// PluginResolver looks up plugins by ID.
type PluginResolver interface {
	// ResolveConnector finds a connector plugin.
	ResolveConnector(id string) (plugins.ConnectorPlugin, error)

	// ResolveProcessor finds a processor plugin.
	ResolveProcessor(id string) (plugins.ProcessorPlugin, error)

	// ResolveOutput finds an output plugin.
	ResolveOutput(id string) (plugins.OutputPlugin, error)
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Methods
// ══════════════════════════════════════════════════════════════════════════════

// NewPipelineRun creates a new pipeline run.
func NewPipelineRun(id string, pipeline *Pipeline) *PipelineRun {
	ctx, cancel := context.WithCancel(context.Background())

	run := &PipelineRun{
		ID:         id,
		PipelineID: pipeline.Name,
		StartTime:  time.Now(),
		State:      PipelineStatePending,
		Jobs:       make(map[string]*JobRun),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initialize job runs
	for jobID := range pipeline.Jobs {
		run.Jobs[jobID] = &JobRun{
			ID:    jobID,
			JobID: jobID,
			State: JobStatePending,
		}
	}

	return run
}

// Cancel cancels the pipeline run.
func (r *PipelineRun) Cancel() {
	if r.cancel != nil {
		r.cancel()
	}
	r.State = PipelineStateCancelled
	r.EndTime = time.Now()
}

// Context returns the run's context.
func (r *PipelineRun) Context() context.Context {
	return r.ctx
}

// Duration returns how long the run took.
func (r *PipelineRun) Duration() time.Duration {
	if r.EndTime.IsZero() {
		return time.Since(r.StartTime)
	}
	return r.EndTime.Sub(r.StartTime)
}

// IsComplete returns true if the run has finished.
func (r *PipelineRun) IsComplete() bool {
	return r.State == PipelineStateCompleted ||
		r.State == PipelineStateFailed ||
		r.State == PipelineStateCancelled
}

// CompletedJobs returns the number of completed jobs.
func (r *PipelineRun) CompletedJobs() int {
	count := 0
	for _, job := range r.Jobs {
		if job.State == JobStateCompleted || job.State == JobStateSkipped {
			count++
		}
	}
	return count
}

// TotalJobs returns the total number of jobs.
func (r *PipelineRun) TotalJobs() int {
	return len(r.Jobs)
}

// Progress returns the overall pipeline progress (0.0 to 1.0).
func (r *PipelineRun) Progress() float64 {
	total := r.TotalJobs()
	if total == 0 {
		return 1.0
	}
	return float64(r.CompletedJobs()) / float64(total)
}
