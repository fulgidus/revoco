package pipeline

import (
	"context"
	"sync"
)

// ══════════════════════════════════════════════════════════════════════════════
// DAG Scheduler
// ══════════════════════════════════════════════════════════════════════════════

// DAGScheduler manages job execution based on dependencies.
type DAGScheduler struct {
	pipeline *Pipeline
	run      *PipelineRun

	mu sync.RWMutex

	// Tracking
	completed map[string]bool // Jobs that have completed (successfully or not)
	failed    map[string]bool // Jobs that failed
	running   map[string]bool // Jobs currently running

	// Channels
	readyCh chan string   // Jobs ready to run
	doneCh  chan jobDone  // Job completion notifications
	errCh   chan error    // Errors
	stopCh  chan struct{} // Stop signal
}

// jobDone represents a job completion notification.
type jobDone struct {
	jobID   string
	success bool
	err     error
}

// NewDAGScheduler creates a new scheduler for the pipeline.
func NewDAGScheduler(pipeline *Pipeline, run *PipelineRun) *DAGScheduler {
	return &DAGScheduler{
		pipeline:  pipeline,
		run:       run,
		completed: make(map[string]bool),
		failed:    make(map[string]bool),
		running:   make(map[string]bool),
		readyCh:   make(chan string, len(pipeline.Jobs)),
		doneCh:    make(chan jobDone, len(pipeline.Jobs)),
		errCh:     make(chan error, 1),
		stopCh:    make(chan struct{}),
	}
}

// Ready returns a channel that emits job IDs when they're ready to run.
func (s *DAGScheduler) Ready() <-chan string {
	return s.readyCh
}

// Done returns a channel for errors.
func (s *DAGScheduler) Errors() <-chan error {
	return s.errCh
}

// Start begins the scheduling loop.
func (s *DAGScheduler) Start(ctx context.Context) {
	// Queue initial jobs (those with no dependencies)
	s.queueReadyJobs()

	// Start the scheduling goroutine
	go s.schedulingLoop(ctx)
}

// Stop stops the scheduler.
func (s *DAGScheduler) Stop() {
	close(s.stopCh)
}

// JobCompleted marks a job as completed.
func (s *DAGScheduler) JobCompleted(jobID string, success bool, err error) {
	s.doneCh <- jobDone{
		jobID:   jobID,
		success: success,
		err:     err,
	}
}

// schedulingLoop runs until all jobs are complete or an error occurs.
func (s *DAGScheduler) schedulingLoop(ctx context.Context) {
	defer close(s.readyCh)

	for {
		select {
		case <-ctx.Done():
			return

		case <-s.stopCh:
			return

		case done := <-s.doneCh:
			s.handleJobDone(done)

			// Check if pipeline is complete
			if s.isComplete() {
				return
			}

			// Queue any jobs that are now ready
			s.queueReadyJobs()
		}
	}
}

// handleJobDone processes a job completion.
func (s *DAGScheduler) handleJobDone(done jobDone) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.running, done.jobID)
	s.completed[done.jobID] = true

	if !done.success {
		s.failed[done.jobID] = true

		// Check if job has continue-on-error
		job := s.pipeline.Jobs[done.jobID]
		if job != nil && !job.ContinueOnError {
			// Mark dependents as skipped
			s.skipDependents(done.jobID)
		}
	}
}

// queueReadyJobs finds jobs whose dependencies are satisfied and queues them.
func (s *DAGScheduler) queueReadyJobs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for jobID, job := range s.pipeline.Jobs {
		// Skip if already completed, running, or queued
		if s.completed[jobID] || s.running[jobID] {
			continue
		}

		// Check if dependencies are satisfied
		if s.dependenciesSatisfied(job) {
			s.running[jobID] = true
			s.readyCh <- jobID
		}
	}
}

// dependenciesSatisfied checks if all dependencies of a job have completed.
func (s *DAGScheduler) dependenciesSatisfied(job *Job) bool {
	for _, need := range job.Needs {
		if !s.completed[need] {
			return false
		}
		// If dependency failed and job doesn't continue on error
		if s.failed[need] {
			needJob := s.pipeline.Jobs[need]
			if needJob == nil || !needJob.ContinueOnError {
				return false
			}
		}
	}
	return true
}

// skipDependents marks all jobs depending on the failed job as skipped.
func (s *DAGScheduler) skipDependents(failedJobID string) {
	visited := make(map[string]bool)
	s.skipDependentsRecursive(failedJobID, visited)
}

func (s *DAGScheduler) skipDependentsRecursive(jobID string, visited map[string]bool) {
	for depID, depJob := range s.pipeline.Jobs {
		if visited[depID] || s.completed[depID] {
			continue
		}

		for _, need := range depJob.Needs {
			if need == jobID {
				visited[depID] = true
				s.completed[depID] = true
				s.failed[depID] = true

				// Update run state
				if jobRun, ok := s.run.Jobs[depID]; ok {
					jobRun.State = JobStateSkipped
					jobRun.Message = "skipped due to failed dependency"
				}

				// Recursively skip dependents
				s.skipDependentsRecursive(depID, visited)
				break
			}
		}
	}
}

// isComplete returns true if all jobs are completed.
func (s *DAGScheduler) isComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for jobID := range s.pipeline.Jobs {
		if !s.completed[jobID] {
			return false
		}
	}
	return true
}

// HasFailures returns true if any jobs failed.
func (s *DAGScheduler) HasFailures() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.failed) > 0
}

// ══════════════════════════════════════════════════════════════════════════════
// Job Dependency Analysis
// ══════════════════════════════════════════════════════════════════════════════

// DependencyGraph represents the job dependency graph.
type DependencyGraph struct {
	// Forward edges: job -> jobs that depend on it
	Dependents map[string][]string

	// Backward edges: job -> jobs it depends on
	Dependencies map[string][]string

	// Topological order
	Order []string
}

// BuildDependencyGraph constructs a dependency graph from a pipeline.
func BuildDependencyGraph(pipeline *Pipeline) (*DependencyGraph, error) {
	graph := &DependencyGraph{
		Dependents:   make(map[string][]string),
		Dependencies: make(map[string][]string),
	}

	// Build edges
	for jobID, job := range pipeline.Jobs {
		graph.Dependencies[jobID] = job.Needs

		for _, need := range job.Needs {
			graph.Dependents[need] = append(graph.Dependents[need], jobID)
		}
	}

	// Compute topological order
	order, err := pipeline.JobOrder()
	if err != nil {
		return nil, err
	}
	graph.Order = order

	return graph, nil
}

// Levels returns jobs grouped by execution level.
// Level 0 = root jobs, Level 1 = jobs depending on Level 0, etc.
func (g *DependencyGraph) Levels() [][]string {
	levels := make(map[string]int)
	maxLevel := 0

	for _, jobID := range g.Order {
		level := 0
		for _, dep := range g.Dependencies[jobID] {
			if depLevel, ok := levels[dep]; ok && depLevel >= level {
				level = depLevel + 1
			}
		}
		levels[jobID] = level
		if level > maxLevel {
			maxLevel = level
		}
	}

	result := make([][]string, maxLevel+1)
	for jobID, level := range levels {
		result[level] = append(result[level], jobID)
	}

	return result
}

// CriticalPath returns the longest path through the graph.
func (g *DependencyGraph) CriticalPath() []string {
	if len(g.Order) == 0 {
		return nil
	}

	// Dynamic programming to find longest path
	dist := make(map[string]int)
	prev := make(map[string]string)

	for _, jobID := range g.Order {
		maxDist := 0
		maxPrev := ""

		for _, dep := range g.Dependencies[jobID] {
			if dist[dep] >= maxDist {
				maxDist = dist[dep] + 1
				maxPrev = dep
			}
		}

		dist[jobID] = maxDist
		if maxPrev != "" {
			prev[jobID] = maxPrev
		}
	}

	// Find the endpoint with maximum distance
	maxJob := ""
	maxDist := -1
	for jobID, d := range dist {
		if d > maxDist {
			maxDist = d
			maxJob = jobID
		}
	}

	// Reconstruct path
	var path []string
	for jobID := maxJob; jobID != ""; jobID = prev[jobID] {
		path = append([]string{jobID}, path...)
	}

	return path
}

// ParallelGroups returns groups of jobs that can run in parallel.
func (g *DependencyGraph) ParallelGroups() [][]string {
	return g.Levels()
}
