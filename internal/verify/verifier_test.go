package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
