package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/sandaogouchen/nep2midsence/internal/config"
	"github.com/sandaogouchen/nep2midsence/internal/tui"
)

// AppOptions are the root-level options passed to the TUI launcher.
type AppOptions struct {
	ConfigPath string
	Verbose    bool
	WorkDir    string
	Version    string
	BuildDate  string
	GitCommit  string
}

func Execute() {
	if err := ExecuteWithArgs(os.Args[1:], launchTUI); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ExecuteWithArgs parses the root flags and launches the TUI.
func ExecuteWithArgs(args []string, launch func(AppOptions) error) error {
	fs := flag.NewFlagSet("nep2midsence", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts AppOptions
	fs.StringVar(&opts.ConfigPath, "config", ".nep2midsence.yaml", "配置文件路径 (默认: .nep2midsence.yaml)")
	fs.BoolVar(&opts.Verbose, "verbose", false, "详细日志输出")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("legacy subcommands are no longer supported; launch the TUI and use slash commands instead")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	opts.WorkDir = wd
	opts.Version = Version
	opts.BuildDate = BuildDate
	opts.GitCommit = GitCommit

	return launch(opts)
}

func launchTUI(opts AppOptions) error {
	cfg, err := loadRuntimeConfig(opts.ConfigPath)
	if err != nil {
		return err
	}

	runtime := tui.NewRuntime(cfg)
	program := tui.NewProgram(cfg, runtime, tui.Options{
		ConfigPath: opts.ConfigPath,
		Verbose:    opts.Verbose,
		WorkDir:    opts.WorkDir,
		Version:    opts.Version,
		BuildDate:  opts.BuildDate,
		GitCommit:  opts.GitCommit,
	})

	_, err = program.Run()
	return err
}

func loadRuntimeConfig(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	cfg = config.DefaultConfig()
	cfg.SourceDir = "."
	cfg.UsePath(path)
	return cfg, nil
}
