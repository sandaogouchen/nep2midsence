package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

func TestCheckCompileSummarizesTypeScriptMissingModules(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake-tsc.sh")
	script := `#!/bin/sh
echo "src/pages/CampaignPage.ts(1,30): error TS2307: Cannot find module '@utils/coreComponents/BaseComponent' or its corresponding type declarations." 1>&2
echo "src/pages/CampaignPage.ts(2,30): error TS2307: Cannot find module '@utils/coreComponents/Radio' or its corresponding type declarations." 1>&2
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}

	v := NewVerifier(dir, scriptPath, "")
	ok, compileErr := v.checkCompile(filepath.Join(dir, "dummy.ts"))
	if ok {
		t.Fatalf("checkCompile should fail for fake TypeScript compile errors")
	}
	if !strings.Contains(compileErr, "missing modules") {
		t.Fatalf("compileErr = %q, want missing modules summary", compileErr)
	}
	if !strings.Contains(compileErr, "BaseComponent") || !strings.Contains(compileErr, "MidComponent") {
		t.Fatalf("compileErr = %q, want BaseComponent replacement suggestion", compileErr)
	}
	if !strings.Contains(compileErr, "Radio") {
		t.Fatalf("compileErr = %q, want missing Radio module included", compileErr)
	}
}

func TestCheckCompileSummarizesTypeScriptMissingModulesFromStdout(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake-tsc-stdout.sh")
	script := `#!/bin/sh
echo "src/pages/CampaignPage.ts(1,30): error TS2307: Cannot find module '@utils/coreComponents/BaseComponent' or its corresponding type declarations."
exit 1
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}

	v := NewVerifier(dir, scriptPath, "")
	ok, compileErr := v.checkCompile(filepath.Join(dir, "dummy.ts"))
	if ok {
		t.Fatalf("checkCompile should fail for fake TypeScript compile errors")
	}
	if !strings.Contains(compileErr, "missing modules") {
		t.Fatalf("compileErr = %q, want missing modules summary", compileErr)
	}
	if !strings.Contains(compileErr, "BaseComponent") {
		t.Fatalf("compileErr = %q, want stdout compile details included", compileErr)
	}
}

func TestSummarizeTypeScriptCompileErrorsSuggestsHandlingSourceLocalAliasImports(t *testing.T) {
	output := `src/case.ts(1,30): error TS2307: Cannot find module '@testData/index' or its corresponding type declarations.
src/case.ts(2,30): error TS2307: Cannot find module '@utils/index' or its corresponding type declarations.
`

	compileErr := summarizeTypeScriptCompileErrors(output)
	if !strings.Contains(compileErr, "@testData/index") {
		t.Fatalf("compileErr = %q, want @testData/index listed", compileErr)
	}
	if !strings.Contains(compileErr, "@utils/index") {
		t.Fatalf("compileErr = %q, want @utils/index listed", compileErr)
	}
	if !strings.Contains(compileErr, "source-local alias import") {
		t.Fatalf("compileErr = %q, want source-local alias suggestion", compileErr)
	}
}

func TestVerifySkipsTestsWhenNoTestCommand(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake-build.sh")
	script := `#!/bin/sh
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}

	targetFile := filepath.Join(dir, "dummy.ts")
	if err := os.WriteFile(targetFile, []byte(`export const ok = true;`), 0o644); err != nil {
		t.Fatalf("WriteFile(target): %v", err)
	}

	v := NewVerifier(dir, scriptPath, "")
	result := &types.MigrationResult{CaseFile: targetFile, TargetFile: targetFile, Success: true}
	vr := v.Verify(result)

	if !vr.CompileOK {
		t.Fatalf("CompileOK = false, want true; CompileError = %q", vr.CompileError)
	}
	if vr.TestOK {
		t.Fatalf("TestOK = true, want false when test command is disabled")
	}
	if vr.TestError != "" {
		t.Fatalf("TestError = %q, want empty when tests are skipped", vr.TestError)
	}
}
