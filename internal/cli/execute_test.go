package cli

import "testing"

func TestExecuteWithArgsLaunchesTUIWithRootFlags(t *testing.T) {
	var got AppOptions

	err := ExecuteWithArgs([]string{"--config", "custom.yaml", "--verbose"}, func(opts AppOptions) error {
		got = opts
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteWithArgs returned error: %v", err)
	}

	if got.ConfigPath != "custom.yaml" {
		t.Fatalf("ConfigPath = %q, want %q", got.ConfigPath, "custom.yaml")
	}
	if !got.Verbose {
		t.Fatalf("Verbose = false, want true")
	}
}

func TestExecuteWithArgsRejectsLegacySubcommands(t *testing.T) {
	err := ExecuteWithArgs([]string{"start", "--dir", "."}, func(AppOptions) error { return nil })
	if err == nil {
		t.Fatal("expected error for legacy subcommand")
	}
}
