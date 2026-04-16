package verify

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// Verifier checks migrated code for correctness
type Verifier struct {
	projectDir string
	buildCmd   string // "go build" / "tsc" etc.
	testCmd    string // "go test" / "npm test" etc.
}

func NewVerifier(projectDir, buildCmd, testCmd string) *Verifier {
	if buildCmd == "" {
		buildCmd = "go build"
	}
	if testCmd == "" {
		testCmd = "go test"
	}
	return &Verifier{
		projectDir: projectDir,
		buildCmd:   buildCmd,
		testCmd:    testCmd,
	}
}

// Verify checks a migrated file for compilation and optionally runs tests
func (v *Verifier) Verify(result *types.MigrationResult) *types.VerifyResult {
	vr := &types.VerifyResult{CaseFile: result.CaseFile}

	// 1. Compile check
	vr.CompileOK, vr.CompileError = v.checkCompile(result.TargetFile)

	// 2. Test run (only if compiles)
	if vr.CompileOK && result.TargetFile != "" {
		vr.TestOK, vr.TestError = v.runTest(result.TargetFile)
	}

	// 3. Generate diff
	if result.CaseFile != "" && result.TargetFile != "" {
		vr.Diff = v.generateDiff(result.CaseFile, result.TargetFile)
	}

	return vr
}

// VerifyAll checks all migration results
func (v *Verifier) VerifyAll(results []*types.MigrationResult) []*types.VerifyResult {
	var verifyResults []*types.VerifyResult
	for _, result := range results {
		if !result.Success {
			verifyResults = append(verifyResults, &types.VerifyResult{
				CaseFile:     result.CaseFile,
				CompileOK:    false,
				CompileError: "migration failed: " + result.Error,
			})
			continue
		}
		vr := v.Verify(result)
		verifyResults = append(verifyResults, vr)
	}
	return verifyResults
}

func (v *Verifier) checkCompile(targetFile string) (bool, string) {
	if targetFile == "" {
		return false, "no target file"
	}

	parts := strings.Fields(v.buildCmd)
	if len(parts) == 0 {
		return false, "empty build command"
	}

	args := append(parts[1:], "./...")
	cmd := exec.Command(parts[0], args...)
	cmd.Dir = v.projectDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, stderr.String()
	}
	return true, ""
}

func (v *Verifier) runTest(targetFile string) (bool, string) {
	parts := strings.Fields(v.testCmd)
	if len(parts) == 0 {
		return false, "empty test command"
	}

	args := append(parts[1:], "-run", ".", "-count=1", "-timeout=60s")
	cmd := exec.Command(parts[0], args...)
	cmd.Dir = v.projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		output := stdout.String() + stderr.String()
		return false, output
	}
	return true, ""
}

func (v *Verifier) generateDiff(sourceFile, targetFile string) string {
	cmd := exec.Command("diff", "-u", sourceFile, targetFile)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Run() // diff returns non-zero when files differ, that's ok
	return stdout.String()
}
