package executor

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCocoExecutorExecuteUsesCocoBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is unix-only")
	}

	tmpDir := t.TempDir()
	writeExecutable(t, filepath.Join(tmpDir, "coco"), "#!/bin/sh\nprintf 'ok\\n'\n")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	exec := NewCocoExecutor(tmpDir, 5*time.Second)
	output, err := exec.Execute(context.Background(), "你是谁")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !output.Success {
		t.Fatalf("output.Success = false, want true")
	}
}

func TestPreflightCheckUsesCocoBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is unix-only")
	}

	tmpDir := t.TempDir()
	writeExecutable(t, filepath.Join(tmpDir, "coco"), "#!/bin/sh\nprintf 'coco version test\\n'\n")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := PreflightCheck(); err != nil {
		t.Fatalf("PreflightCheck returned error: %v", err)
	}
}

func TestCocoExecutorExecuteBuildsSupportedCLIArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is unix-only")
	}

	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "args.txt")
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$@\" > " + shellQuote(argsPath),
		"printf 'ok\\n'",
	}, "\n") + "\n"
	writeExecutable(t, filepath.Join(tmpDir, "coco"), script)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	exec := NewCocoExecutor(tmpDir, 5*time.Second)
	if _, err := exec.Execute(context.Background(), "你是谁"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", argsPath, err)
	}

	got := strings.Fields(string(argsData))
	want := []string{
		"-p",
		"你是谁",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestCocoExecutorExecuteAllowsCLIToExitAfterQueryTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is unix-only")
	}

	tmpDir := t.TempDir()
	script := strings.Join([]string{
		"#!/bin/sh",
		"sleep 1.2",
		"printf 'ok\\n'",
	}, "\n") + "\n"
	writeExecutable(t, filepath.Join(tmpDir, "coco"), script)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	exec := NewCocoExecutor(tmpDir, 1*time.Second)
	output, err := exec.Execute(context.Background(), "延迟退出")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !output.Success {
		t.Fatal("output.Success = false, want true")
	}
}

func TestCocoExecutorExecuteReturnsErrorOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is unix-only")
	}

	tmpDir := t.TempDir()
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf 'some output\\n'",
		"printf 'backend failed\\n' >&2",
		"exit 1",
	}, "\n") + "\n"
	writeExecutable(t, filepath.Join(tmpDir, "coco"), script)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	exec := NewCocoExecutor(tmpDir, 5*time.Second)
	output, err := exec.Execute(context.Background(), "失败场景")
	if err == nil {
		t.Fatal("Execute returned nil error, want failure")
	}
	if output == nil {
		t.Fatal("output is nil")
	}
	if output.Success {
		t.Fatal("output.Success = true, want false")
	}
	if !strings.Contains(err.Error(), "backend failed") {
		t.Fatalf("error = %q, want stderr in error message", err.Error())
	}
}

func TestCCExecutorUsesClaudePrintFlagAndPositionalPrompt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is unix-only")
	}

	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "claude_args.txt")
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$@\" > " + shellQuote(argsPath),
		"printf 'ok\\n'",
	}, "\n") + "\n"
	writeExecutable(t, filepath.Join(tmpDir, "claude"), script)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	exec := NewCCExecutor(tmpDir, 5*time.Second)
	if _, err := exec.Execute(context.Background(), "你是谁"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", argsPath, err)
	}
	got := strings.Fields(string(argsData))
	want := []string{"-p", "你是谁"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestCodexExecutorUsesExecAndDoesNotUseProfileFlagForPrompt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test is unix-only")
	}

	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "codex_args.txt")
	script := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$@\" > " + shellQuote(argsPath),
		"printf 'ok\\n'",
	}, "\n") + "\n"
	writeExecutable(t, filepath.Join(tmpDir, "codex"), script)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	exec := NewCodexExecutor(tmpDir, 5*time.Second)
	if _, err := exec.Execute(context.Background(), "hello"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", argsPath, err)
	}
	got := strings.Fields(string(argsData))
	want := []string{"exec", "hello"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func writeExecutable(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}
