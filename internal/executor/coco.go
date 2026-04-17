package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// CocoExecutor wraps invocations of the coco CLI binary.
type CocoExecutor struct {
	workDir string
	timeout time.Duration
}

// PromptExecutor is the minimal interface needed by the scheduler.
// It executes a single prompt via an external CLI and returns its output.
type PromptExecutor interface {
	Execute(ctx context.Context, prompt string) (*types.CocoOutput, error)
}

const (
	ToolCoco  = "coco"
	ToolCC    = "cc"    // Claude Code
	ToolCodex = "codex" // OpenAI Codex
)

// NormalizeToolForConfig converts user/config input into a supported tool identifier.
// It is intentionally forgiving and accepts common aliases.
func NormalizeToolForConfig(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "", "coco":
		return ToolCoco
	case "cc", "claude", "claude-code", "claudecode", "claude_code":
		return ToolCC
	case "codex":
		return ToolCodex
	default:
		return strings.ToLower(strings.TrimSpace(tool))
	}
}

// NewCocoExecutor creates a new CocoExecutor
func NewCocoExecutor(workDir string, timeout time.Duration) *CocoExecutor {
	return &CocoExecutor{
		workDir: workDir,
		timeout: timeout,
	}
}

// Execute runs a single Coco invocation with the given prompt
func (c *CocoExecutor) Execute(ctx context.Context, prompt string) (*types.CocoOutput, error) {
	args := []string{"-p", prompt}

	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.commandTimeout())
		defer cancel()
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, "coco", args...)
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
		return output, c.formatExecuteError(ctx, err, stderr.String())
	}

	output.Success = true
	output.ExitCode = 0

	return output, nil
}

func (c *CocoExecutor) commandTimeout() time.Duration {
	if c.timeout <= 0 {
		return 0
	}

	grace := c.timeout / 5
	if grace < 5*time.Second {
		grace = 5 * time.Second
	}
	if grace > 30*time.Second {
		grace = 30 * time.Second
	}

	return c.timeout + grace
}

func (c *CocoExecutor) formatExecuteError(ctx context.Context, err error, stderr string) error {
	parts := []string{fmt.Sprintf("coco execution failed: %v", err)}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		parts = append(parts, fmt.Sprintf("local-guard-timeout=%s", c.commandTimeout()))
	}

	trimmedStderr := strings.TrimSpace(stderr)
	if trimmedStderr != "" {
		parts = append(parts, fmt.Sprintf("stderr: %s", trimmedStderr))
	}

	return errors.New(strings.Join(parts, "\n"))
}

// PreflightCheck verifies that the coco CLI is available and functional.
func PreflightCheck() error {
	return PreflightCheckForTool(ToolCoco)
}

// PreflightCheckForTool verifies that the selected external CLI exists and is runnable.
// It tries common version/help flags to be resilient across different CLIs.
func PreflightCheckForTool(tool string) error {
	bin := toolBinary(NormalizeToolForConfig(tool))
	for _, args := range [][]string{{"--version"}, {"-v"}, {"-h"}, {"--help"}} {
		cmd := exec.Command(bin, args...)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("%s CLI not found or not functional", bin)
}

func toolBinary(tool string) string {
	switch NormalizeToolForConfig(tool) {
	case ToolCC:
		return "claude"
	case ToolCodex:
		return "codex"
	default:
		if strings.TrimSpace(tool) == "" {
			return ToolCoco
		}
		return tool
	}
}

// ---------------------------------------------------------------------------
// Alternate CLIs (Claude Code / Codex)
// ---------------------------------------------------------------------------

type claudeExecutor struct {
	workDir string
	timeout time.Duration
}

func (e *claudeExecutor) Execute(ctx context.Context, prompt string) (*types.CocoOutput, error) {
	// Claude Code: `claude -p <prompt>`
	args := []string{"-p", prompt}
	return runSimpleExecutor(ctx, "claude", e.workDir, e.timeout, args)
}

type codexExecutor struct {
	workDir string
	timeout time.Duration
}

func (e *codexExecutor) Execute(ctx context.Context, prompt string) (*types.CocoOutput, error) {
	// Codex: `codex exec [PROMPT]` (note: `-p` is profile, not prompt)
	args := []string{"exec", prompt}
	return runSimpleExecutor(ctx, "codex", e.workDir, e.timeout, args)
}

func runSimpleExecutor(ctx context.Context, bin string, workDir string, timeout time.Duration, args []string) (*types.CocoOutput, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = workDir

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
		trimmedStderr := strings.TrimSpace(stderr.String())
		if trimmedStderr != "" {
			return output, fmt.Errorf("%s execution failed: %v\nstderr: %s", bin, err, trimmedStderr)
		}
		return output, fmt.Errorf("%s execution failed: %v", bin, err)
	}

	output.Success = true
	output.ExitCode = 0
	return output, nil
}

func NewCCExecutor(workDir string, timeout time.Duration) PromptExecutor {
	return &claudeExecutor{workDir: workDir, timeout: timeout}
}

func NewCodexExecutor(workDir string, timeout time.Duration) PromptExecutor {
	return &codexExecutor{workDir: workDir, timeout: timeout}
}
