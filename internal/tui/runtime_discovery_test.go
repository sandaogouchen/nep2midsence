package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandaogouchen/nep2midsence/internal/analyzer"
	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/types"
)

func TestCollectLocalImportDepsResolvesTsConfigAlias(t *testing.T) {
	dir := t.TempDir()

	targetFile := filepath.Join(dir, "e2e", "pages", "new_pages", "campaignPage", "components", "WebsiteRadio.ts")
	if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(targetFile, []byte("export class WebsiteRadio {}"), 0o644); err != nil {
		t.Fatalf("WriteFile(target): %v", err)
	}

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@pages/*": ["./e2e/pages/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	tscp, err := config.LoadTsConfig(tsconfigPath)
	if err != nil {
		t.Fatalf("LoadTsConfig: %v", err)
	}

	a := &types.FullAnalysis{
		FilePath: filepath.Join(dir, "e2e", "tests", "case.spec.ts"),
		AST: &types.ASTInfo{
			Imports: []types.ImportInfo{
				{Path: "@pages/new_pages/campaignPage/components/WebsiteRadio"},
			},
		},
	}

	got := collectLocalImportDeps(a, tscp)
	if len(got) != 1 || got[0] != targetFile {
		t.Fatalf("collectLocalImportDeps() = %v, want [%q]", got, targetFile)
	}
}

func TestResolveModuleFileFromPropertySupportsAliasImports(t *testing.T) {
	dir := t.TempDir()

	moduleFile := filepath.Join(dir, "e2e", "pages", "new_pages", "campaignPage", "components", "WebsiteRadio.ts")
	if err := os.MkdirAll(filepath.Dir(moduleFile), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(moduleFile, []byte("export class WebsiteRadio {}"), 0o644); err != nil {
		t.Fatalf("WriteFile(module): %v", err)
	}

	pageFile := filepath.Join(dir, "e2e", "pages", "CampaignPage.ts")
	if err := os.MkdirAll(filepath.Dir(pageFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(page): %v", err)
	}
	pageSource := `import { WebsiteRadio } from "@pages/new_pages/campaignPage/components/WebsiteRadio";

export class CampaignPage {
  websiteRadio: WebsiteRadio;
}`
	if err := os.WriteFile(pageFile, []byte(pageSource), 0o644); err != nil {
		t.Fatalf("WriteFile(page): %v", err)
	}

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@pages/*": ["./e2e/pages/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	tscp, err := config.LoadTsConfig(tsconfigPath)
	if err != nil {
		t.Fatalf("LoadTsConfig: %v", err)
	}

	got := resolveModuleFileFromProperty(pageFile, "websiteRadio", tscp)
	if got != moduleFile {
		t.Fatalf("resolveModuleFileFromProperty() = %q, want %q", got, moduleFile)
	}
}

func TestResolveModuleFileFromPropertySupportsDefiniteAssignmentTypes(t *testing.T) {
	dir := t.TempDir()

	moduleFile := filepath.Join(dir, "e2e", "pages", "new_pages", "campaignPage", "module", "CampaignNameModule", "CampaignNameModule.ts")
	if err := os.MkdirAll(filepath.Dir(moduleFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(module): %v", err)
	}
	if err := os.WriteFile(moduleFile, []byte("export class CampaignNameModule {}"), 0o644); err != nil {
		t.Fatalf("WriteFile(module): %v", err)
	}

	pageFile := filepath.Join(dir, "e2e", "pages", "new_pages", "campaignPage", "CampaignPage.ts")
	if err := os.MkdirAll(filepath.Dir(pageFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(page): %v", err)
	}
	pageSource := `import { CampaignNameModule } from "@pages/new_pages/campaignPage/module/CampaignNameModule/CampaignNameModule";

export class CampaignPage {
  campaignNameModule!: CampaignNameModule;
}`
	if err := os.WriteFile(pageFile, []byte(pageSource), 0o644); err != nil {
		t.Fatalf("WriteFile(page): %v", err)
	}

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@pages/*": ["./e2e/pages/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	tscp, err := config.LoadTsConfig(tsconfigPath)
	if err != nil {
		t.Fatalf("LoadTsConfig: %v", err)
	}

	got := resolveModuleFileFromProperty(pageFile, "campaignNameModule", tscp)
	if got != moduleFile {
		t.Fatalf("resolveModuleFileFromProperty() = %q, want %q", got, moduleFile)
	}
}

func TestScanExtendedDirectoriesIncludesInheritedNepFiles(t *testing.T) {
	dir := t.TempDir()

	baseFile := filepath.Join(dir, "e2e", "utils", "coreComponents", "BaseComponent.ts")
	radioFile := filepath.Join(dir, "e2e", "utils", "coreComponents", "Radio.ts")
	childFile := filepath.Join(dir, "e2e", "pages", "new_pages", "campaignPage", "components", "WebsiteRadio.ts")

	for _, path := range []string{baseFile, radioFile, childFile} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
	}

	if err := os.WriteFile(baseFile, []byte(`export class BaseComponent {
  async tap() { return ai.action("tap"); }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(base): %v", err)
	}
	if err := os.WriteFile(radioFile, []byte(`import { BaseComponent } from "@utils/coreComponents/BaseComponent";
export class Radio extends BaseComponent {}`), 0o644); err != nil {
		t.Fatalf("WriteFile(radio): %v", err)
	}
	if err := os.WriteFile(childFile, []byte(`import { Radio } from "@utils/coreComponents/Radio";
export class WebsiteRadio extends Radio {}`), 0o644); err != nil {
		t.Fatalf("WriteFile(child): %v", err)
	}

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if err := os.WriteFile(tsconfigPath, []byte(`{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@utils/*": ["./e2e/utils/*"]
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	tscp, err := config.LoadTsConfig(tsconfigPath)
	if err != nil {
		t.Fatalf("LoadTsConfig: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Source.Dir = dir
	cfg.ScanDirectories = []string{"e2e/pages", "e2e/utils/coreComponents"}

	graph := analyzer.BuildInheritanceGraph(dir, tscp)
	got := scanExtendedDirectories(cfg, tscp, graph)
	if len(got) == 0 {
		t.Fatalf("scanExtendedDirectories() returned no candidates")
	}

	found := false
	for _, path := range got {
		if path == childFile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("scanExtendedDirectories() = %v, want to include %q", got, childFile)
	}
}
