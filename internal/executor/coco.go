package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.commandTimeout())
		defer cancel()
	}

	// coco 在非 TTY 环境下可能无法正确启用其“操作文件/交互能力”。
	// 如果环境支持 tmux，则将 `coco -p` 放到 tmux pane 里执行，以获得真实 TTY。
	if shouldRunCocoInTmux() {
		return c.executeCocoPromptInTmux(ctx, prompt)
	}
	return c.executeCocoPromptDirect(ctx, prompt)
}

func (c *CocoExecutor) executeCocoPromptDirect(ctx context.Context, prompt string) (*types.CocoOutput, error) {
	args := []string{"-p", prompt}

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

func shouldRunCocoInTmux() bool {
	// Allow disabling tmux wrapper explicitly (useful for tests and constrained environments).
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("NEP2MIDSENCE_COCO_TMUX"))); v != "" {
		switch v {
		case "0", "false", "off", "no":
			return false
		case "1", "true", "on", "yes":
			// fallthrough to capability check
		default:
			// Unknown value: be conservative and keep default behavior (auto-detect).
		}
	}
	_, err := exec.LookPath("tmux")
	return err == nil
}

func (c *CocoExecutor) executeCocoPromptInTmux(ctx context.Context, prompt string) (*types.CocoOutput, error) {
	start := time.Now()

	outFile, err := os.CreateTemp("", "nep2midsence-coco-tmux-out-*")
	if err != nil {
		return c.executeCocoPromptDirect(ctx, prompt)
	}
	outPath := outFile.Name()
	_ = outFile.Close()
	defer func() { _ = os.Remove(outPath) }()

	exitFile, err := os.CreateTemp("", "nep2midsence-coco-tmux-exit-*")
	if err != nil {
		return c.executeCocoPromptDirect(ctx, prompt)
	}
	exitPath := exitFile.Name()
	_ = exitFile.Close()
	defer func() { _ = os.Remove(exitPath) }()

	// Use a unique session + wait-for tokens to avoid cross-task collisions when scheduler runs in parallel.
	session := fmt.Sprintf("nep2midsence-coco-%d-%d", time.Now().UnixNano(), os.Getpid())
	startToken := session + "-start"
	doneToken := session + "-done"
	// Capture pane id from tmux to avoid user-configured base-index differences.
	var paneTarget string

	// Start a detached session that blocks until we finish wiring pipe-pane.
	script := strings.Join([]string{
		// wait for start signal from the parent process
		"tmux wait-for \"$5\" || exit 125",
		// ensure working directory
		"cd \"$1\" || exit 126",
		// run coco prompt
		"coco -p \"$2\"",
		// capture exit code
		"code=$?; printf '%d' \"$code\" >\"$3\"",
		// signal completion
		"tmux wait-for -S \"$4\"",
	}, "; ")

	newArgs := []string{"new-session", "-d", "-P", "-F", "#{pane_id}", "-s", session, "sh", "-c", script, "sh", c.workDir, prompt, exitPath, doneToken, startToken}
	cmd := exec.CommandContext(ctx, "tmux", newArgs...)
	var paneID bytes.Buffer
	cmd.Stdout = &paneID
	if err := cmd.Run(); err != nil {
		return c.executeCocoPromptDirect(ctx, prompt)
	}
	paneTarget = strings.TrimSpace(paneID.String())
	if paneTarget == "" {
		return c.executeCocoPromptDirect(ctx, prompt)
	}

	// Best-effort cleanup even when ctx is cancelled.
	defer func() { _ = exec.Command("tmux", "kill-session", "-t", session).Run() }()
	defer func() { _ = exec.Command("tmux", "pipe-pane", "-t", paneTarget).Run() }()

	// Pipe pane output to a file while keeping coco attached to a real TTY.
	pipeCmd := "cat >> " + shellQuotePath(outPath)
	if err := exec.CommandContext(ctx, "tmux", "pipe-pane", "-t", paneTarget, "-o", pipeCmd).Run(); err != nil {
		// If piping fails, fall back to direct execution to avoid hanging.
		return c.executeCocoPromptDirect(ctx, prompt)
	}

	// Signal session to start and then wait for completion.
	if err := exec.CommandContext(ctx, "tmux", "wait-for", "-S", startToken).Run(); err != nil {
		return c.executeCocoPromptDirect(ctx, prompt)
	}

	waitErr := exec.CommandContext(ctx, "tmux", "wait-for", doneToken).Run()
	duration := time.Since(start)

	outBytes, _ := os.ReadFile(outPath)
	combined := string(outBytes)

	output := &types.CocoOutput{Output: combined, Duration: duration}
	// If we failed to wait due to timeout/cancel, return as executor failure.
	if waitErr != nil {
		output.Success = false
		output.ExitCode = 124
		_ = exec.Command("tmux", "kill-session", "-t", session).Run()
		return output, c.formatExecuteError(ctx, waitErr, combined)
	}

	exitData, readErr := os.ReadFile(exitPath)
	if readErr != nil {
		output.Success = false
		output.ExitCode = 1
		return output, c.formatExecuteError(ctx, readErr, combined)
	}
	codeStr := strings.TrimSpace(string(exitData))
	code, convErr := strconv.Atoi(codeStr)
	if convErr != nil {
		output.Success = false
		output.ExitCode = 1
		return output, c.formatExecuteError(ctx, convErr, combined)
	}

	if code != 0 {
		output.Success = false
		output.ExitCode = code
		return output, c.formatExecuteError(ctx, fmt.Errorf("exit code %d", code), combined)
	}

	output.Success = true
	output.ExitCode = 0
	return output, nil
}

func shellQuotePath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
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
