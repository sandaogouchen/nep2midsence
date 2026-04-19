package verify

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
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
	return &Verifier{
		projectDir: projectDir,
		buildCmd:   buildCmd,
		testCmd:    testCmd,
	}
}

// Verify checks a migrated file for compilation and optionally runs tests
func (v *Verifier) Verify(result *types.MigrationResult) *types.VerifyResult {
	vr := &types.VerifyResult{CaseFile: result.CaseFile}

	// 0. NEP residual check (cross-repo: migrated files must not contain NEP markers)
	vr.NepCleanOK, vr.NepCleanError = CheckNepClean(result.TargetFile)

	// 1. Compile check
	vr.CompileOK, vr.CompileError = v.CheckCompile(result.TargetFile)

	// 2. Test run (only if compiles and test execution is enabled)
	if vr.CompileOK && result.TargetFile != "" && strings.TrimSpace(v.testCmd) != "" {
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

func (v *Verifier) CheckCompile(targetFile string) (bool, string) {
	return v.checkCompile(targetFile)
}

func (v *Verifier) checkCompile(targetFile string) (bool, string) {
	if targetFile == "" {
		return false, "no target file"
	}

	parts := strings.Fields(v.buildCmd)
	if len(parts) == 0 {
		return false, "empty build command"
	}

	args := buildCompileArgs(parts)
	cmd := exec.Command(parts[0], args...)
	cmd.Dir = v.projectDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		output := stderr.String()
		if strings.TrimSpace(output) == "" {
			output = err.Error()
		}
		return false, summarizeTypeScriptCompileErrors(output)
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

// nepResidualMarkers are strings that indicate NEP framework usage.
var nepResidualMarkers = []string{
	"ai.action(",
	"ai?.action(",
	"ai.getElement(",
	"ai?.getElement(",
	"clickElementByVL(",
	"from 'nep",
	`from "nep`,
	"require('nep",
	`require("nep`,
	"nep_utils",
	"AiAgent",
}

// CheckNepClean scans a target file for residual NEP markers.
// Returns (true, "") if clean, or (false, details) if residual markers are found.
func CheckNepClean(targetFile string) (bool, string) {
	if targetFile == "" {
		return true, ""
	}
	content, err := os.ReadFile(targetFile)
	if err != nil {
		// File doesn't exist yet (migration may have failed); skip check.
		return true, ""
	}
	text := string(content)
	var found []string
	for _, marker := range nepResidualMarkers {
		if strings.Contains(text, marker) {
			found = append(found, marker)
		}
	}
	if len(found) == 0 {
		return true, ""
	}
	return false, fmt.Sprintf("NEP residual markers found: %s", strings.Join(found, ", "))
}

func buildCompileArgs(parts []string) []string {
	if len(parts) <= 1 {
		return nil
	}
	cmdName := filepathBase(parts[0])
	if cmdName == "go" && len(parts) > 1 && parts[1] == "build" {
		return append(parts[1:], "./...")
	}
	return parts[1:]
}

var tsMissingModuleRe = regexp.MustCompile(`TS2307: Cannot find module ['"]([^'"]+)['"]`)

func summarizeTypeScriptCompileErrors(output string) string {
	matches := tsMissingModuleRe.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return output
	}

	seen := make(map[string]struct{})
	var modules []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		moduleName := strings.TrimSpace(match[1])
		if moduleName == "" {
			continue
		}
		if _, ok := seen[moduleName]; ok {
			continue
		}
		seen[moduleName] = struct{}{}
		modules = append(modules, moduleName)
	}
	sort.Strings(modules)

	lines := []string{
		fmt.Sprintf("TypeScript compile failed: missing modules (%d)", len(modules)),
		strings.Join(modules, ", "),
	}
	for _, moduleName := range modules {
		if suggestion := suggestReplacementForMissingModule(moduleName); suggestion != "" {
			lines = append(lines, suggestion)
		}
	}
	return strings.Join(lines, "\n")
}

func suggestReplacementForMissingModule(moduleName string) string {
	switch {
	case strings.Contains(moduleName, "BaseComponent"):
		return "Suggestion: replace BaseComponent references with MidComponent compatibility exports."
	case strings.Contains(moduleName, "Radio"):
		return "Suggestion: provide MidRadio compatibility exports for Radio-based components."
	case strings.Contains(moduleName, "Input"):
		return "Suggestion: provide MidInput compatibility exports for Input-based components."
	case strings.Contains(moduleName, "Switch"):
		return "Suggestion: provide MidSwitch compatibility exports for Switch-based components."
	case strings.HasPrefix(moduleName, "@testData/"), strings.HasPrefix(moduleName, "@utils/"), strings.HasPrefix(moduleName, "@pages/"), strings.HasPrefix(moduleName, "@constants/"), strings.HasPrefix(moduleName, "@conf/"):
		return "Suggestion: source-local alias import leaked into cross-repo output; read the source dependency and inline the minimal exported constants/mock/helper code, or rewrite the import to a target-local resolvable file."
	default:
		return ""
	}
}

func filepathBase(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
