package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/plugins"
)

// ══════════════════════════════════════════════════════════════════════════════
// Default Executor
// ══════════════════════════════════════════════════════════════════════════════

// DefaultExecutor is the standard pipeline executor implementation.
type DefaultExecutor struct {
	resolver PluginResolver

	// Configuration
	maxParallel int           // Maximum parallel jobs
	jobTimeout  time.Duration // Default job timeout
	stepTimeout time.Duration // Default step timeout

	// State
	mu   sync.RWMutex
	runs map[string]*PipelineRun
}

// ExecutorOption configures the executor.
type ExecutorOption func(*DefaultExecutor)

// WithMaxParallel sets the maximum number of parallel jobs.
func WithMaxParallel(n int) ExecutorOption {
	return func(e *DefaultExecutor) {
		e.maxParallel = n
	}
}

// WithJobTimeout sets the default job timeout.
func WithJobTimeout(d time.Duration) ExecutorOption {
	return func(e *DefaultExecutor) {
		e.jobTimeout = d
	}
}

// WithStepTimeout sets the default step timeout.
func WithStepTimeout(d time.Duration) ExecutorOption {
	return func(e *DefaultExecutor) {
		e.stepTimeout = d
	}
}

// NewExecutor creates a new pipeline executor.
func NewExecutor(resolver PluginResolver, opts ...ExecutorOption) *DefaultExecutor {
	e := &DefaultExecutor{
		resolver:    resolver,
		maxParallel: 4,
		jobTimeout:  1 * time.Hour,
		stepTimeout: 30 * time.Minute,
		runs:        make(map[string]*PipelineRun),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// ══════════════════════════════════════════════════════════════════════════════
// Executor Interface Implementation
// ══════════════════════════════════════════════════════════════════════════════

// Execute runs a pipeline and returns when complete.
func (e *DefaultExecutor) Execute(ctx context.Context, pipeline *Pipeline, progress ProgressCallback) (*PipelineRun, error) {
	// Generate run ID
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())

	// Create run
	run := NewPipelineRun(runID, pipeline)

	// Store run
	e.mu.Lock()
	e.runs[runID] = run
	e.mu.Unlock()

	// Execute
	err := e.executePipeline(ctx, pipeline, run, progress)

	// Update final state
	run.EndTime = time.Now()
	if err != nil {
		run.State = PipelineStateFailed
		run.Message = err.Error()
	} else {
		run.State = PipelineStateCompleted
	}

	// Final progress update
	if progress != nil {
		event := EventPipelineCompleted
		if run.State == PipelineStateFailed {
			event = EventPipelineFailed
		}
		progress(PipelineProgress{
			Run:     run,
			Event:   event,
			Message: run.Message,
		})
	}

	return run, err
}

// Cancel stops a running pipeline.
func (e *DefaultExecutor) Cancel(runID string) error {
	e.mu.RLock()
	run, ok := e.runs[runID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("run not found: %s", runID)
	}

	run.Cancel()
	return nil
}

// Status returns the current status of a pipeline run.
func (e *DefaultExecutor) Status(runID string) (*PipelineRun, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	run, ok := e.runs[runID]
	if !ok {
		return nil, fmt.Errorf("run not found: %s", runID)
	}

	return run, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Pipeline Execution
// ══════════════════════════════════════════════════════════════════════════════

func (e *DefaultExecutor) executePipeline(ctx context.Context, pipeline *Pipeline, run *PipelineRun, progress ProgressCallback) error {
	// Use run's context for cancellation
	ctx = run.Context()

	// Update state
	run.State = PipelineStateRunning

	// Notify start
	if progress != nil {
		progress(PipelineProgress{
			Run:     run,
			Event:   EventPipelineStarted,
			Message: fmt.Sprintf("Starting pipeline: %s", pipeline.Name),
		})
	}

	// Create scheduler
	scheduler := NewDAGScheduler(pipeline, run)
	scheduler.Start(ctx)
	defer scheduler.Stop()

	// Create semaphore for parallel job limiting
	sem := make(chan struct{}, e.maxParallel)

	// Create wait group for jobs
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	// Process ready jobs
	for jobID := range scheduler.Ready() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		jobID := jobID // Capture for goroutine
		job := pipeline.Jobs[jobID]
		jobRun := run.Jobs[jobID]

		// Acquire semaphore
		sem <- struct{}{}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			// Execute job
			success, err := e.executeJob(ctx, job, jobRun, run, progress)

			// Record first error
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
				})
			}

			// Notify scheduler
			scheduler.JobCompleted(jobID, success, err)
		}()
	}

	// Wait for all jobs
	wg.Wait()

	// Check for failures
	if scheduler.HasFailures() {
		if firstErr != nil {
			return firstErr
		}
		return fmt.Errorf("one or more jobs failed")
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Job Execution
// ══════════════════════════════════════════════════════════════════════════════

func (e *DefaultExecutor) executeJob(ctx context.Context, job *Job, jobRun *JobRun, run *PipelineRun, progress ProgressCallback) (bool, error) {
	// Set timeout
	timeout := e.jobTimeout
	if job.Timeout.Duration() > 0 {
		timeout = job.Timeout.Duration()
	}

	jobCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Update state
	jobRun.State = JobStateRunning
	jobRun.StartTime = time.Now()

	// Notify start
	if progress != nil {
		progress(PipelineProgress{
			Run:     run,
			Job:     jobRun,
			Event:   EventJobStarted,
			Message: fmt.Sprintf("Starting job: %s", job.Name),
		})
	}

	// Get input items from dependencies
	jobRun.InputItems = e.collectJobInputs(job, run)

	// Initialize step runs
	jobRun.Steps = make([]*StepRun, len(job.Steps))
	for i, step := range job.Steps {
		jobRun.Steps[i] = &StepRun{
			ID:        fmt.Sprintf("%s-%d", jobRun.ID, i),
			StepID:    step.ID,
			StepIndex: i,
			State:     StepStatePending,
		}
	}

	// Execute steps sequentially
	currentItems := jobRun.InputItems
	var stepErr error

	for i, step := range job.Steps {
		stepRun := jobRun.Steps[i]

		outputItems, err := e.executeStep(jobCtx, step, stepRun, currentItems, run, jobRun, progress)
		if err != nil {
			stepErr = err
			if !step.ContinueOnError {
				break
			}
		}

		// Update items for next step
		if outputItems != nil {
			currentItems = outputItems
		}
	}

	// Update state
	jobRun.EndTime = time.Now()
	jobRun.OutputItems = currentItems

	if stepErr != nil {
		jobRun.State = JobStateFailed
		jobRun.Message = stepErr.Error()

		if progress != nil {
			progress(PipelineProgress{
				Run:     run,
				Job:     jobRun,
				Event:   EventJobFailed,
				Message: fmt.Sprintf("Job failed: %s - %v", job.Name, stepErr),
			})
		}

		if job.ContinueOnError {
			return true, nil // Treat as success for dependency purposes
		}
		return false, stepErr
	}

	jobRun.State = JobStateCompleted

	if progress != nil {
		progress(PipelineProgress{
			Run:     run,
			Job:     jobRun,
			Event:   EventJobCompleted,
			Message: fmt.Sprintf("Job completed: %s", job.Name),
		})
	}

	return true, nil
}

// collectJobInputs gathers output items from dependency jobs.
func (e *DefaultExecutor) collectJobInputs(job *Job, run *PipelineRun) []*core.DataItem {
	if len(job.Needs) == 0 {
		// Root job starts with no items (connector will fetch)
		return nil
	}

	// Collect outputs from all dependencies
	var items []*core.DataItem
	for _, depID := range job.Needs {
		if depRun, ok := run.Jobs[depID]; ok {
			items = append(items, depRun.OutputItems...)
		}
	}

	return items
}

// ══════════════════════════════════════════════════════════════════════════════
// Step Execution
// ══════════════════════════════════════════════════════════════════════════════

func (e *DefaultExecutor) executeStep(ctx context.Context, step *Step, stepRun *StepRun, items []*core.DataItem, run *PipelineRun, jobRun *JobRun, progress ProgressCallback) ([]*core.DataItem, error) {
	// Set timeout
	timeout := e.stepTimeout
	if step.Timeout.Duration() > 0 {
		timeout = step.Timeout.Duration()
	}

	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Update state
	stepRun.State = StepStateRunning
	stepRun.StartTime = time.Now()
	stepRun.InputItems = items

	// Notify start
	if progress != nil {
		progress(PipelineProgress{
			Run:     run,
			Job:     jobRun,
			Step:    stepRun,
			Event:   EventStepStarted,
			Message: fmt.Sprintf("Starting step: %s", step.Name),
		})
	}

	// Determine step type and execute
	outputItems, err := e.executeStepPlugin(stepCtx, step, stepRun, items, run, jobRun, progress)

	// Update state
	stepRun.EndTime = time.Now()
	stepRun.OutputItems = outputItems

	if err != nil {
		stepRun.State = StepStateFailed
		stepRun.Message = err.Error()

		if progress != nil {
			progress(PipelineProgress{
				Run:     run,
				Job:     jobRun,
				Step:    stepRun,
				Event:   EventStepFailed,
				Message: fmt.Sprintf("Step failed: %s - %v", step.Name, err),
			})
		}

		return items, err // Return original items on failure
	}

	stepRun.State = StepStateCompleted

	if progress != nil {
		progress(PipelineProgress{
			Run:     run,
			Job:     jobRun,
			Step:    stepRun,
			Event:   EventStepCompleted,
			Message: fmt.Sprintf("Step completed: %s", step.Name),
		})
	}

	return outputItems, nil
}

func (e *DefaultExecutor) executeStepPlugin(ctx context.Context, step *Step, stepRun *StepRun, items []*core.DataItem, run *PipelineRun, jobRun *JobRun, progress ProgressCallback) ([]*core.DataItem, error) {
	// Try to resolve as connector first
	if connector, err := e.resolver.ResolveConnector(step.Uses); err == nil {
		return e.executeConnectorStep(ctx, step, stepRun, connector, items, run, jobRun, progress)
	}

	// Try to resolve as processor
	if processor, err := e.resolver.ResolveProcessor(step.Uses); err == nil {
		return e.executeProcessorStep(ctx, step, stepRun, processor, items, run, jobRun, progress)
	}

	// Try to resolve as output
	if output, err := e.resolver.ResolveOutput(step.Uses); err == nil {
		return e.executeOutputStep(ctx, step, stepRun, output, items, run, jobRun, progress)
	}

	return nil, fmt.Errorf("plugin not found: %s", step.Uses)
}

// ══════════════════════════════════════════════════════════════════════════════
// Connector Step Execution
// ══════════════════════════════════════════════════════════════════════════════

func (e *DefaultExecutor) executeConnectorStep(ctx context.Context, step *Step, stepRun *StepRun, plugin plugins.ConnectorPlugin, items []*core.DataItem, run *PipelineRun, jobRun *JobRun, progress ProgressCallback) ([]*core.DataItem, error) {
	connector := plugin.AsConnector()

	// Get reader if available
	reader, hasReader := plugin.AsReader()
	if !hasReader {
		return nil, fmt.Errorf("connector %s does not support reading", step.Uses)
	}

	// Build config
	config := core.ConnectorConfig{
		ConnectorID: connector.ID(),
		Settings:    step.With,
	}

	// Initialize
	if err := reader.Initialize(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to initialize connector: %w", err)
	}
	defer reader.Close()

	// List items
	progressFn := func(current, total int) {
		if progress != nil {
			stepRun.ItemsProcessed = current
			stepRun.ItemsTotal = total
			progress(PipelineProgress{
				Run:      run,
				Job:      jobRun,
				Step:     stepRun,
				Event:    EventStepProgress,
				Progress: float64(current) / float64(max(total, 1)),
				Message:  fmt.Sprintf("Listing items: %d/%d", current, total),
			})
		}
	}

	listedItems, err := reader.List(ctx, progressFn)
	if err != nil {
		return nil, fmt.Errorf("failed to list items: %w", err)
	}

	// Convert to pointers
	outputItems := make([]*core.DataItem, len(listedItems))
	for i := range listedItems {
		outputItems[i] = &listedItems[i]
	}

	return outputItems, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Processor Step Execution
// ══════════════════════════════════════════════════════════════════════════════

func (e *DefaultExecutor) executeProcessorStep(ctx context.Context, step *Step, stepRun *StepRun, plugin plugins.ProcessorPlugin, items []*core.DataItem, run *PipelineRun, jobRun *JobRun, progress ProgressCallback) ([]*core.DataItem, error) {
	processor := plugin.AsProcessor()

	// Filter items by selector
	filteredItems := items
	if step.Selector != nil {
		matcher, err := plugins.NewSelectorMatcher(step.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid selector: %w", err)
		}

		filteredItems = make([]*core.DataItem, 0, len(items))
		for _, item := range items {
			if matcher.Match(item) && processor.CanProcess(item) {
				filteredItems = append(filteredItems, item)
			}
		}
	} else {
		// Use processor's default selector
		filteredItems = make([]*core.DataItem, 0, len(items))
		for _, item := range items {
			if processor.CanProcess(item) {
				filteredItems = append(filteredItems, item)
			}
		}
	}

	stepRun.ItemsTotal = len(filteredItems)

	// Process items
	progressFn := func(done, total int, message string) {
		if progress != nil {
			stepRun.ItemsProcessed = done
			progress(PipelineProgress{
				Run:      run,
				Job:      jobRun,
				Step:     stepRun,
				Event:    EventStepProgress,
				Progress: float64(done) / float64(max(total, 1)),
				Message:  message,
			})
		}
	}

	processedItems, err := processor.ProcessBatch(ctx, filteredItems, step.With, progressFn)
	if err != nil {
		return nil, fmt.Errorf("failed to process items: %w", err)
	}

	// Merge processed items with unprocessed items
	outputItems := mergeItems(items, filteredItems, processedItems)

	return outputItems, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Output Step Execution
// ══════════════════════════════════════════════════════════════════════════════

func (e *DefaultExecutor) executeOutputStep(ctx context.Context, step *Step, stepRun *StepRun, plugin plugins.OutputPlugin, items []*core.DataItem, run *PipelineRun, jobRun *JobRun, progress ProgressCallback) ([]*core.DataItem, error) {
	// Output steps don't modify items, they just export them
	// The actual export logic would go here

	// For now, just pass through items
	return items, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Functions
// ══════════════════════════════════════════════════════════════════════════════

// mergeItems combines original items with processed items.
// Items that were processed are replaced with their processed versions.
// Items that were not processed are kept as-is.
func mergeItems(original, filtered, processed []*core.DataItem) []*core.DataItem {
	// Create a map of processed items by ID
	processedMap := make(map[string]*core.DataItem)
	for _, item := range processed {
		processedMap[item.ID] = item
	}

	// Create a set of filtered item IDs
	filteredSet := make(map[string]bool)
	for _, item := range filtered {
		filteredSet[item.ID] = true
	}

	// Build output
	result := make([]*core.DataItem, 0, len(original))
	for _, item := range original {
		if filteredSet[item.ID] {
			// Item was processed
			if processedItem, ok := processedMap[item.ID]; ok {
				result = append(result, processedItem)
			}
			// If not in processedMap, item was filtered out
		} else {
			// Item was not processed, keep original
			result = append(result, item)
		}
	}

	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
