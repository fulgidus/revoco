package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ══════════════════════════════════════════════════════════════════════════════
// Pipeline Parsing
// ══════════════════════════════════════════════════════════════════════════════

// ParseFile loads a pipeline from a YAML file.
func ParseFile(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pipeline file: %w", err)
	}

	pipeline, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pipeline %s: %w", path, err)
	}

	pipeline.SourcePath = path
	return pipeline, nil
}

// Parse parses a pipeline from YAML bytes.
func Parse(data []byte) (*Pipeline, error) {
	var pipeline Pipeline
	if err := yaml.Unmarshal(data, &pipeline); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	// Validate and normalize
	if err := pipeline.Validate(); err != nil {
		return nil, err
	}

	pipeline.Normalize()

	return &pipeline, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Validation
// ══════════════════════════════════════════════════════════════════════════════

// Validate checks if the pipeline is valid.
func (p *Pipeline) Validate() error {
	if p.Name == "" {
		return &ValidationError{Field: "name", Message: "pipeline name is required"}
	}

	if len(p.Jobs) == 0 {
		return &ValidationError{Field: "jobs", Message: "at least one job is required"}
	}

	// Validate each job
	jobIDs := make(map[string]bool)
	for id, job := range p.Jobs {
		jobIDs[id] = true
		job.ID = id

		if err := job.Validate(); err != nil {
			return &ValidationError{
				Field:   fmt.Sprintf("jobs.%s", id),
				Message: err.Error(),
			}
		}
	}

	// Validate job dependencies exist
	for id, job := range p.Jobs {
		for _, need := range job.Needs {
			if !jobIDs[need] {
				return &ValidationError{
					Field:   fmt.Sprintf("jobs.%s.needs", id),
					Message: fmt.Sprintf("job %q depends on non-existent job %q", id, need),
				}
			}
			if need == id {
				return &ValidationError{
					Field:   fmt.Sprintf("jobs.%s.needs", id),
					Message: fmt.Sprintf("job %q cannot depend on itself", id),
				}
			}
		}
	}

	// Check for circular dependencies
	if err := p.detectCycles(); err != nil {
		return err
	}

	return nil
}

// Validate checks if a job is valid.
func (j *Job) Validate() error {
	if len(j.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	// Validate each step
	stepIDs := make(map[string]bool)
	for i, step := range j.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}

		// Check for duplicate step IDs
		if step.ID != "" {
			if stepIDs[step.ID] {
				return fmt.Errorf("duplicate step ID: %q", step.ID)
			}
			stepIDs[step.ID] = true
		}
	}

	return nil
}

// Validate checks if a step is valid.
func (s *Step) Validate() error {
	if s.Uses == "" {
		return fmt.Errorf("'uses' is required")
	}

	return nil
}

// detectCycles checks for circular dependencies in job graph.
func (p *Pipeline) detectCycles() error {
	// Build adjacency list
	graph := make(map[string][]string)
	for id, job := range p.Jobs {
		graph[id] = job.Needs
	}

	// DFS to detect cycles
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string, path []string) error
	dfs = func(node string, path []string) error {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, neighbor := range graph[node] {
			if !visited[neighbor] {
				if err := dfs(neighbor, path); err != nil {
					return err
				}
			} else if recStack[neighbor] {
				// Found cycle
				cycleStart := 0
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				cycle := append(path[cycleStart:], neighbor)
				return &ValidationError{
					Field:   "jobs",
					Message: fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " -> ")),
				}
			}
		}

		recStack[node] = false
		return nil
	}

	for id := range p.Jobs {
		if !visited[id] {
			if err := dfs(id, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Normalization
// ══════════════════════════════════════════════════════════════════════════════

// Normalize fills in default values and generates IDs.
func (p *Pipeline) Normalize() {
	for id, job := range p.Jobs {
		job.ID = id
		job.Normalize()
	}
}

// Normalize fills in default values for a job.
func (j *Job) Normalize() {
	if j.Name == "" {
		j.Name = j.ID
	}

	for i, step := range j.Steps {
		step.Normalize(i)
	}
}

// Normalize fills in default values for a step.
func (s *Step) Normalize(index int) {
	if s.ID == "" {
		s.ID = fmt.Sprintf("step-%d", index)
	}

	if s.Name == "" {
		s.Name = s.Uses
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Error Types
// ══════════════════════════════════════════════════════════════════════════════

// ValidationError represents a pipeline validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error in %s: %s", e.Field, e.Message)
}

// ══════════════════════════════════════════════════════════════════════════════
// Pipeline Discovery
// ══════════════════════════════════════════════════════════════════════════════

// DiscoverPipelines finds all pipeline files in a directory.
func DiscoverPipelines(dir string) ([]*Pipeline, error) {
	var pipelines []*Pipeline

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read pipeline directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, name)
		pipeline, err := ParseFile(path)
		if err != nil {
			// Log warning but continue
			continue
		}

		pipelines = append(pipelines, pipeline)
	}

	return pipelines, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Methods
// ══════════════════════════════════════════════════════════════════════════════

// JobOrder returns jobs in topological order (dependencies first).
func (p *Pipeline) JobOrder() ([]string, error) {
	// Build in-degree map
	inDegree := make(map[string]int)
	for id := range p.Jobs {
		inDegree[id] = 0
	}
	for _, job := range p.Jobs {
		for _, need := range job.Needs {
			inDegree[job.ID]++
			_ = need // need is the dependency, job.ID depends on it
		}
	}

	// Find all jobs with no dependencies
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	// Process queue (Kahn's algorithm)
	var order []string
	for len(queue) > 0 {
		// Pop from queue
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)

		// Reduce in-degree for dependents
		for depID, depJob := range p.Jobs {
			for _, need := range depJob.Needs {
				if need == id {
					inDegree[depID]--
					if inDegree[depID] == 0 {
						queue = append(queue, depID)
					}
				}
			}
		}
	}

	if len(order) != len(p.Jobs) {
		return nil, fmt.Errorf("cycle detected in job dependencies")
	}

	return order, nil
}

// RootJobs returns jobs with no dependencies.
func (p *Pipeline) RootJobs() []string {
	var roots []string
	for id, job := range p.Jobs {
		if len(job.Needs) == 0 {
			roots = append(roots, id)
		}
	}
	return roots
}

// Dependents returns jobs that depend on the given job.
func (p *Pipeline) Dependents(jobID string) []string {
	var deps []string
	for id, job := range p.Jobs {
		for _, need := range job.Needs {
			if need == jobID {
				deps = append(deps, id)
				break
			}
		}
	}
	return deps
}

// Clone creates a deep copy of the pipeline.
func (p *Pipeline) Clone() *Pipeline {
	data, _ := yaml.Marshal(p)
	var clone Pipeline
	yaml.Unmarshal(data, &clone)
	clone.SourcePath = p.SourcePath
	return &clone
}
