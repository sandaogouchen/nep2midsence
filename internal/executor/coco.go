package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// CocoExecutor wraps invocations of the Coco (Claude Code CLI) binary
type CocoExecutor struct {
	workDir      string
	maxTurns     int
	allowedTools []string
	timeout      time.Duration
	outputFormat string
}

// NewCocoExecutor creates a new CocoExecutor
func NewCocoExecutor(workDir string, maxTurns int, allowedTools []string, timeout time.Duration, outputFormat string) *CocoExecutor {
	if maxTurns <= 0 {
		maxTurns = 10
	}
	if outputFormat == "" {
		outputFormat = "json"
	}
	return &CocoExecutor{
		workDir:      workDir,
		maxTurns:     maxTurns,
		allowedTools: allowedTools,
		timeout:      timeout,
		outputFormat: outputFormat,
	}
}

// Execute runs a single Coco invocation with the given prompt
func (c *CocoExecutor) Execute(ctx context.Context, prompt string) (*types.CocoOutput, error) {
	args := []string{
		"--print",
		fmt.Sprintf("--max-turns=%d", c.maxTurns),
		fmt.Sprintf("--output-format=%s", c.outputFormat),
	}

	for _, tool := range c.allowedTools {
		args = append(args, fmt.Sprintf("--allowedTools=%s", tool))
	}

	args = append(args, prompt)

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = c.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	output := &types.CocoOutput{
		Output:   stdout.String(),
		Duration: duration,
	}

	if err != nil {
		output.Success = false
		if exitErr, ok := err.(*exec.ExitError); ok {
			output.ExitCode = exitErr.ExitCode()
		}
		return output, fmt.Errorf("coco execution failed: %w\nstderr: %s", err, stderr.String())
	}

	output.Success = true
	output.ExitCode = 0

	// Try to parse JSON output for session ID and cost
	if c.outputFormat == "json" {
		var parsed map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &parsed); err == nil {
			if sid, ok := parsed["session_id"].(string); ok {
				output.SessionID = sid
			}
			if cost, ok := parsed["cost_usd"].(float64); ok {
				output.CostUSD = cost
			}
		}
	}

	return output, nil
}

// PreflightCheck verifies that the Coco CLI is available and functional
func PreflightCheck() error {
	cmd := exec.Command("claude", "--version")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude CLI not found or not functional: %w (is Claude Code CLI installed?)", err)
	}
	return nil
}
