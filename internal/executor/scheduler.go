package executor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/prompt"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// ProgressCallback is called after each task completes.
type ProgressCallback func(result *types.MigrationResult, current, total int)

// Scheduler orchestrates parallel execution of migration tasks
type Scheduler struct {
	executor   PromptExecutor
	generator  *prompt.Generator
	maxJobs    int
	retryLimit int
	onProgress ProgressCallback
}

// NewScheduler creates a new Scheduler
func NewScheduler(executor PromptExecutor, generator *prompt.Generator, maxJobs, retryLimit int) *Scheduler {
	if maxJobs <= 0 {
		maxJobs = 1
	}
	if retryLimit < 0 {
		retryLimit = 0
	}
	return &Scheduler{
		executor:   executor,
		generator:  generator,
		maxJobs:    maxJobs,
		retryLimit: retryLimit,
	}
}

// SetProgressCallback registers a callback invoked after each task
func (s *Scheduler) SetProgressCallback(cb ProgressCallback) {
	s.onProgress = cb
}

// Run executes all analyses through the Coco pipeline, returning migration results
func (s *Scheduler) Run(ctx context.Context, analyses []*types.FullAnalysis) []*types.MigrationResult {
	total := len(analyses)
	results := make([]*types.MigrationResult, total)

	sem := make(chan struct{}, s.maxJobs)
	var mu sync.Mutex
	var wg sync.WaitGroup
	completed := 0

	for i, analysis := range analyses {
		wg.Add(1)
		go func(idx int, a *types.FullAnalysis) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := s.executeOne(ctx, a)
			mu.Lock()
			results[idx] = result
			completed++
			current := completed
			mu.Unlock()

			if s.onProgress != nil {
				s.onProgress(result, current, total)
			}
		}(i, analysis)
	}

	wg.Wait()
	return results
}

func (s *Scheduler) executeOne(ctx context.Context, analysis *types.FullAnalysis) *types.MigrationResult {
	result := &types.MigrationResult{
		CaseFile:   analysis.FilePath,
		TargetFile: analysis.TargetPath,
	}

	promptText, err := s.generator.Generate(analysis)
	if err != nil {
		result.Error = fmt.Sprintf("prompt generation failed: %v", err)
		return result
	}

	var lastErr error
	for attempt := 0; attempt <= s.retryLimit; attempt++ {
		result.RetryCount = attempt

		start := time.Now()
		output, err := s.executor.Execute(ctx, promptText)
		result.Duration = time.Since(start)

		if err == nil && output.Success {
			result.Success = true
			result.Output = output.Output
			return result
		}

		lastErr = err
		if output != nil {
			result.Output = output.Output
		}
	}

	result.Error = fmt.Sprintf("all attempts failed: %v", lastErr)
	return result
}
