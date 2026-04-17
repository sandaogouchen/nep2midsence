package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSupportsFlatWrapperFilterAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".nep2midsence.yaml")
	content := `known_infra_roots: page, browser, context
force_infra_calls: |
  .*\.editAdSubmitBtn\.click$
  .*\.sessionStorage\.set$
force_business_calls: .*\.commonActions\..+
force_infra_methods: waitForPageLoadStable, waitForNetworkIdle
element_like_properties: Btn, Button, Input
infra_terminal_methods: click, fill, locator
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	assertContains(t, cfg.WrapperFilter.KnownInfraRoots, "page")
	assertContains(t, cfg.WrapperFilter.KnownInfraRoots, "browser")
	assertContains(t, cfg.WrapperFilter.ForceInfraCallPatterns, `.*\.editAdSubmitBtn\.click$`)
	assertContains(t, cfg.WrapperFilter.ForceInfraCallPatterns, `.*\.sessionStorage\.set$`)
	assertContains(t, cfg.WrapperFilter.ForceBusinessCallPatterns, `.*\.commonActions\..+`)
	assertContains(t, cfg.WrapperFilter.ForceInfraMethods, "waitForPageLoadStable")
	assertContains(t, cfg.WrapperFilter.ForceInfraMethods, "waitForNetworkIdle")
	assertContains(t, cfg.WrapperFilter.ElementLikePropertyPatterns, "Btn")
	assertContains(t, cfg.WrapperFilter.InfraTerminalMethods, "click")
	assertContains(t, cfg.WrapperFilter.InfraTerminalMethods, "locator")

	if cfg.KnownInfraRoots == "" || cfg.ForceInfraCalls == "" || cfg.ForceInfraMethods == "" {
		t.Fatalf("flat alias fields should be normalized back onto config struct")
	}
}

func TestDefaultConfigLoadsBuiltInWrapperFilter(t *testing.T) {
	cfg := DefaultConfig()

	assertContains(t, cfg.WrapperFilter.KnownInfraRoots, "page")
	assertContains(t, cfg.WrapperFilter.ForceInfraMethods, "waitForPageLoadStable")
	if !strings.Contains(cfg.ForceInfraMethods, "waitForPageLoadStable") {
		t.Fatalf("ForceInfraMethods alias should include embedded defaults, got %q", cfg.ForceInfraMethods)
	}
}

func TestDefaultConfigBackfillsFlatAliasFields(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.KnownInfraRoots == "" {
		t.Fatalf("KnownInfraRoots alias should be prefilled from defaults")
	}
	if cfg.ForceInfraMethods == "" {
		t.Fatalf("ForceInfraMethods alias should be prefilled from defaults")
	}
	if cfg.InfraTerminalMethods == "" {
		t.Fatalf("InfraTerminalMethods alias should be prefilled from defaults")
	}
}

func assertContains(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Fatalf("%q not found in %v", want, items)
}
