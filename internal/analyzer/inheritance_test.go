package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

func TestExtractTypeScriptFallbackCapturesInheritanceMetadata(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "WebsiteRadio.ts")
	source := `import { Radio } from "./Radio";

export class WebsiteRadio extends Radio {
  async choose() {
    return this.click();
  }
}
`
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	astInfo, _, _, err := extractTypeScriptFallback(filePath, config.DefaultConfig())
	if err != nil {
		t.Fatalf("extractTypeScriptFallback returned error: %v", err)
	}

	if astInfo.ClassName != "WebsiteRadio" {
		t.Fatalf("ClassName = %q, want %q", astInfo.ClassName, "WebsiteRadio")
	}
	if astInfo.ExtendsFrom != "Radio" {
		t.Fatalf("ExtendsFrom = %q, want %q", astInfo.ExtendsFrom, "Radio")
	}
	if astInfo.ExtendsImport != "./Radio" {
		t.Fatalf("ExtendsImport = %q, want %q", astInfo.ExtendsImport, "./Radio")
	}
}

func TestBuildInheritanceGraphMarksInheritedNepFiles(t *testing.T) {
	dir := t.TempDir()

	baseFile := filepath.Join(dir, "BaseComponent.ts")
	radioFile := filepath.Join(dir, "Radio.ts")
	childFile := filepath.Join(dir, "WebsiteRadio.ts")

	if err := os.WriteFile(baseFile, []byte(`export class BaseComponent {
  async tap() { return ai.action("tap"); }
}`), 0o644); err != nil {
		t.Fatalf("WriteFile(base): %v", err)
	}

	if err := os.WriteFile(radioFile, []byte(`import { BaseComponent } from "./BaseComponent";
export class Radio extends BaseComponent {}`), 0o644); err != nil {
		t.Fatalf("WriteFile(radio): %v", err)
	}

	if err := os.WriteFile(childFile, []byte(`import { Radio } from "./Radio";
export class WebsiteRadio extends Radio {}`), 0o644); err != nil {
		t.Fatalf("WriteFile(child): %v", err)
	}

	graph := BuildInheritanceGraph(dir, nil)
	if graph == nil {
		t.Fatalf("BuildInheritanceGraph returned nil")
	}

	if !graph.IsNepRelated(childFile) {
		t.Fatalf("expected child file to be marked NEP-related via inheritance")
	}

	chain := graph.GetNepAncestorChain(childFile)
	if !strings.Contains(chain, "WebsiteRadio") || !strings.Contains(chain, "BaseComponent") {
		t.Fatalf("GetNepAncestorChain(%q) = %q, want readable chain", childFile, chain)
	}
}
