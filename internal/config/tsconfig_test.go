package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigIncludesImportChainSettings(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.EnableImportChain {
		t.Fatalf("EnableImportChain should default to true")
	}
	if cfg.ImportChainMaxDepth != 20 {
		t.Fatalf("ImportChainMaxDepth = %d, want 20", cfg.ImportChainMaxDepth)
	}
	if len(cfg.ScanDirectories) == 0 {
		t.Fatalf("ScanDirectories should provide default scan roots")
	}
}

func TestLoadTsConfigResolvesWildcardAlias(t *testing.T) {
	dir := t.TempDir()
	pagesDir := filepath.Join(dir, "e2e", "pages", "new_pages", "campaignPage", "components")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	targetFile := filepath.Join(pagesDir, "WebsiteRadio.ts")
	if err := os.WriteFile(targetFile, []byte("export class WebsiteRadio {}"), 0o644); err != nil {
		t.Fatalf("WriteFile(targetFile): %v", err)
	}

	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	tsconfig := `{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@pages/*": ["./e2e/pages/*"],
      "@utils/*": ["./e2e/utils/*"]
    }
  }
}`
	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0o644); err != nil {
		t.Fatalf("WriteFile(tsconfig): %v", err)
	}

	tscp, err := LoadTsConfig(tsconfigPath)
	if err != nil {
		t.Fatalf("LoadTsConfig returned error: %v", err)
	}

	importPath := "@pages/new_pages/campaignPage/components/WebsiteRadio"
	if !tscp.CanResolve(importPath) {
		t.Fatalf("CanResolve(%q) = false, want true", importPath)
	}

	resolved := tscp.Resolve(importPath)
	if len(resolved) == 0 {
		t.Fatalf("Resolve(%q) returned no candidates", importPath)
	}
	if resolved[0] != targetFile {
		t.Fatalf("Resolve(%q) first candidate = %q, want %q", importPath, resolved[0], targetFile)
	}
}

func TestLoadTsConfigMergesRelativeExtends(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "tsconfig.base.json")
	baseContent := `{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@pages/*": ["./e2e/pages/*"]
    }
  }
}`
	if err := os.WriteFile(basePath, []byte(baseContent), 0o644); err != nil {
		t.Fatalf("WriteFile(base): %v", err)
	}

	childPath := filepath.Join(dir, "tsconfig.json")
	childContent := `{
  "extends": "./tsconfig.base.json",
  "compilerOptions": {
    "paths": {
      "@utils/*": ["./e2e/utils/*"]
    }
  }
}`
	if err := os.WriteFile(childPath, []byte(childContent), 0o644); err != nil {
		t.Fatalf("WriteFile(child): %v", err)
	}

	tscp, err := LoadTsConfig(childPath)
	if err != nil {
		t.Fatalf("LoadTsConfig returned error: %v", err)
	}

	if !tscp.CanResolve("@pages/foo") {
		t.Fatalf("expected inherited @pages alias to be available")
	}
	if !tscp.CanResolve("@utils/bar") {
		t.Fatalf("expected child @utils alias to be available")
	}
}
