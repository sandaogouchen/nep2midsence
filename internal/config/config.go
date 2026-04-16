package config

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the full configuration for the migration tool
type Config struct {
	Source         SourceConfig              `json:"source" yaml:"source"`
	Target         TargetConfig              `json:"target" yaml:"target"`
	Analysis       AnalysisConfig            `json:"analysis" yaml:"analysis"`
	CustomMappings map[string]*CustomMapping `json:"custom_mappings" yaml:"custom_mappings"`
	Coco           CocoConfig                `json:"coco" yaml:"coco"`
	Execution      ExecutionConfig           `json:"execution" yaml:"execution"`
	MigrationDoc   string                    `json:"migration_doc" yaml:"migration_doc"`
}

type SourceConfig struct {
	FilePatterns    []string `json:"file_patterns" yaml:"file_patterns"`
	PackagePrefixes []string `json:"package_prefixes" yaml:"package_prefixes"`
}

type TargetConfig struct {
	OutputDir  string `json:"output_dir" yaml:"output_dir"`
	FileSuffix string `json:"file_suffix" yaml:"file_suffix"`
}

type AnalysisConfig struct {
	MaxCallDepth   int  `json:"max_call_depth" yaml:"max_call_depth"`
	EnableDataflow bool `json:"enable_dataflow" yaml:"enable_dataflow"`
	EnableIntent   bool `json:"enable_intent" yaml:"enable_intent"`
}

type CustomMapping struct {
	MidsceneEquivalent string `json:"midscene_equivalent" yaml:"midscene_equivalent"`
	NeedsIntentRewrite bool   `json:"needs_intent_rewrite" yaml:"needs_intent_rewrite"`
}

type CocoConfig struct {
	MaxTurns     int      `json:"max_turns" yaml:"max_turns"`
	AllowedTools []string `json:"allowed_tools" yaml:"allowed_tools"`
	Timeout      string   `json:"timeout" yaml:"timeout"`
	OutputFormat string   `json:"output_format" yaml:"output_format"`
}

type ExecutionConfig struct {
	RetryLimit int `json:"retry_limit" yaml:"retry_limit"`
	MaxJobs    int `json:"max_jobs" yaml:"max_jobs"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Source: SourceConfig{
			FilePatterns:    []string{"*_test.go"},
			PackagePrefixes: []string{"nep"},
		},
		Target: TargetConfig{
			OutputDir:  "midscene",
			FileSuffix: "_midscene",
		},
		Analysis: AnalysisConfig{
			MaxCallDepth:   5,
			EnableDataflow: true,
			EnableIntent:   true,
		},
		Coco: CocoConfig{
			MaxTurns:     10,
			AllowedTools: []string{"Read", "Write", "Edit", "Bash"},
			Timeout:      "3m",
			OutputFormat: "json",
		},
		Execution: ExecutionConfig{
			RetryLimit: 2,
			MaxJobs:    1,
		},
	}
}

// Load reads config from a file path; if empty, tries .nep2midsence.yaml then falls back to defaults
func Load(path string) (*Config, error) {
	if path == "" {
		// Try default locations (new name first, legacy fallback)
		candidates := []string{
			".nep2midsence.yaml", ".nep2midsence.yml", ".nep2midsence.json",
			".casemover.yaml", ".casemover.yml", ".casemover.json",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
	}

	cfg := DefaultConfig()
	if path == "" {
		// No config file found, use defaults
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	// Try YAML first, then JSON
	if err := yaml.Unmarshal(data, cfg); err != nil {
		if err2 := json.Unmarshal(data, cfg); err2 != nil {
			return nil, fmt.Errorf("parsing config file %s: yaml: %w, json: %v", path, err, err2)
		}
	}

	return cfg, nil
}
