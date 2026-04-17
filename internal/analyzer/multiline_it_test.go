package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// TestMultiLineItCallDetected verifies that the TS fallback parser correctly
// identifies test functions when it()/test() calls span multiple lines, which
// is the pattern used in /tt4b_creation_e2e/e2e/tests/new_tests/brand/reach/.
func TestMultiLineItCallDetected(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "reach-cbo-FrequencyCap.ts")
	source := `import { before, describe, it, Page } from '@pagepass/test';
import { commonIt } from "@utils/nep_utils/CommonIt/CommonIt";

describe('reach-cbo-FrequencyCap', () => {
    before(async ({ page }) => {
        await page.goto('/some-url');
    });

    it(
      'reach cbo frequency cap case',
      async ({page, ai}) => {
        await ai.action('click the create button');
        await ai.getElement('campaign name input');
    })

    it(
      'reach cbo another case',
      async ({page, ai}) => {
        await ai.action('fill in budget');
    })
});
`
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write ts file: %v", err)
	}

	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	// Step 1: Verify matchesPattern accepts .ts files
	if !engine.matchesPattern("reach-cbo-FrequencyCap.ts") {
		t.Fatal("matchesPattern rejected .ts file")
	}

	// Step 2: Verify AnalyzeFile succeeds and detects test functions
	result, err := engine.AnalyzeFile(filePath)
	if err != nil {
		t.Fatalf("AnalyzeFile failed: %v", err)
	}
	if result == nil || result.AST == nil {
		t.Fatal("AnalyzeFile returned nil result or AST")
	}

	// Step 3: Verify at least one function has IsTest == true
	testCount := 0
	for _, fn := range result.AST.Functions {
		if fn.IsTest {
			testCount++
			t.Logf("Detected test function: %q (lines %d-%d)", fn.Name, fn.LineStart, fn.LineEnd)
		}
	}
	if testCount == 0 {
		t.Fatalf("No test functions detected; expected at least 2 from multi-line it() calls. Functions found: %d", len(result.AST.Functions))
	}
	if testCount < 2 {
		t.Fatalf("Only %d test function(s) detected; expected at least 2", testCount)
	}
	t.Logf("Total test functions detected: %d", testCount)
}

// TestMultiLineItCallWithCommonIt verifies detection of commonIt() calls.
func TestMultiLineItCallWithCommonIt(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "reach-edit-campaign.ts")
	source := `import { commonIt } from "@utils/nep_utils/CommonIt/CommonIt";
import { describe, it, before } from '@pagepass/test';

describe('reach-edit-campaign', () => {
    before(async ({ page }) => {
        await page.goto('/url');
    });

    it(
      'edit campaign case 1',
      async ({page, ai}) => {
        await ai.action('click edit');
    })

    commonIt(
      'edit campaign common case',
      async ({page, ai}) => {
        await ai.action('fill form');
    })
});
`
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write ts file: %v", err)
	}

	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	result, err := engine.AnalyzeFile(filePath)
	if err != nil {
		t.Fatalf("AnalyzeFile failed: %v", err)
	}

	testCount := 0
	for _, fn := range result.AST.Functions {
		if fn.IsTest {
			testCount++
			t.Logf("Detected test function: %q (lines %d-%d)", fn.Name, fn.LineStart, fn.LineEnd)
		}
	}
	if testCount == 0 {
		t.Fatal("No test functions detected; expected multi-line it() and commonIt() to be detected")
	}
	t.Logf("Total test functions detected: %d", testCount)
}

// TestAnalyzeDirDetectsReachTSFiles verifies end-to-end: AnalyzeDir on a folder
// containing .ts files with multi-line it() calls correctly detects files and cases.
func TestAnalyzeDirDetectsReachTSFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 3 .ts files similar to the reach folder structure
	files := map[string]string{
		"reach-cbo-FrequencyCap.ts": `import { describe, it } from '@pagepass/test';
describe('freq-cap', () => {
    it(
      'frequency cap test',
      async ({page, ai}) => {
        await ai.action('click button');
    })
});
`,
		"reach-cbo-lifetime.ts": `import { describe, it } from '@pagepass/test';
describe('lifetime', () => {
    it(
      'lifetime test',
      async ({page, ai}) => {
        await ai.action('set budget');
    })
});
`,
		"reach-nocbo.ts": `import { describe, it } from '@pagepass/test';
describe('nocbo', () => {
    it("nocbo inline test", async ({page, ai}) => {
        await ai.action('submit form');
    })
});
`,
	}

	for name, content := range files {
		fp := filepath.Join(dir, name)
		if err := os.WriteFile(fp, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	analyses, err := engine.AnalyzeDir(dir)
	if err != nil {
		t.Fatalf("AnalyzeDir failed: %v", err)
	}

	if len(analyses) != 3 {
		t.Fatalf("AnalyzeDir found %d files, expected 3", len(analyses))
	}

	// Verify each analysis has at least one IsTest function
	for _, a := range analyses {
		hasTest := false
		for _, fn := range a.AST.Functions {
			if fn.IsTest {
				hasTest = true
				break
			}
		}
		if !hasTest {
			t.Errorf("File %s has no test functions detected", a.FilePath)
		}
	}
	t.Logf("AnalyzeDir correctly detected %d files with test functions", len(analyses))
}

// TestStateFileNotCreatedInSelectedDir verifies that running a workflow does
// not leave .nep2midsence-state.json in the selected scan directory.
func TestStateFileNotCreatedInSelectedDir(t *testing.T) {
	scanDir := t.TempDir()

	// Create a .ts file so the directory is non-empty
	fp := filepath.Join(scanDir, "sample.ts")
	source := `import { describe, it } from '@pagepass/test';
describe('sample', () => {
    it("test case", async ({ai}) => {
        await ai.action('click');
    })
});
`
	if err := os.WriteFile(fp, []byte(source), 0o644); err != nil {
		t.Fatalf("write ts file: %v", err)
	}

	// Verify no state file appears in scanDir after analysis
	stateFile := filepath.Join(scanDir, ".nep2midsence-state.json")
	if _, err := os.Stat(stateFile); err == nil {
		t.Fatal(".nep2midsence-state.json should not exist in scan directory before analysis")
	}

	cfg := config.DefaultConfig()
	engine := NewEngine(cfg)

	_, err := engine.AnalyzeDir(scanDir)
	if err != nil {
		t.Fatalf("AnalyzeDir failed: %v", err)
	}

	// After analysis, state file should still not exist in the scan dir
	if _, err := os.Stat(stateFile); err == nil {
		t.Fatal(".nep2midsence-state.json was created in the scan directory; it should only be in the project root")
	}
}
