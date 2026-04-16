package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = ".nep2midsence.yaml"

type Config struct {
	Source         SourceConfig      `json:"source" yaml:"source"`
	Target         TargetConfig      `json:"target" yaml:"target"`
	Analysis       AnalysisConfig    `json:"analysis" yaml:"analysis"`
	CustomMappings map[string]string `json:"custom_mappings" yaml:"custom_mappings"`
	Coco           CocoConfig        `json:"coco" yaml:"coco"`
	Execution      ExecutionConfig   `json:"execution" yaml:"execution"`
	MigrationDoc   string            `json:"migration_doc" yaml:"migration_doc"`
	TSExtractor    TSExtractorConfig `json:"ts_extractor" yaml:"ts_extractor"`
	Fingerprint    FingerprintConfig `json:"fingerprint" yaml:"fingerprint"`
	SourceDir      string            `json:"source_dir" yaml:"source_dir"`
	Workers        int               `json:"workers" yaml:"workers"`
	configPath     string            `json:"-" yaml:"-"`
}

type SourceConfig struct {
	Dir             string            `json:"dir" yaml:"dir"`
	Extensions      []string          `json:"extensions" yaml:"extensions"`
	Pattern         string            `json:"pattern" yaml:"pattern"`
	Exclude         []string          `json:"exclude" yaml:"exclude"`
	Language        string            `json:"language" yaml:"language"`
	FilePatterns    []string          `json:"file_patterns" yaml:"file_patterns"`
	PackagePrefixes []string          `json:"package_prefixes" yaml:"package_prefixes"`
	CustomMappings  map[string]string `json:"custom_mappings" yaml:"custom_mappings"`
}

type TargetConfig struct {
	OutputDir  string `json:"output_dir" yaml:"output_dir"`
	FileSuffix string `json:"file_suffix" yaml:"file_suffix"`
}

type AnalysisConfig struct {
	MaxCallDepth   int  `json:"max_call_depth" yaml:"max_call_depth"`
	MaxFiles       int  `json:"max_files" yaml:"max_files"`
	EnableDataflow bool `json:"enable_dataflow" yaml:"enable_dataflow"`
	EnableIntent   bool `json:"enable_intent" yaml:"enable_intent"`
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

type TSExtractorConfig struct {
	NodePath   string `json:"node_path" yaml:"node_path"`
	ScriptPath string `json:"script_path" yaml:"script_path"`
	Timeout    int    `json:"timeout" yaml:"timeout"`
}

type FingerprintConfig struct {
	CustomMappings map[string]string `json:"custom_mappings" yaml:"custom_mappings"`
}

func DefaultConfig() *Config {
	return &Config{
		Source: SourceConfig{
			Dir: ".", Extensions: []string{".ts"}, Pattern: "**/*",
			Exclude: []string{"node_modules", "dist"}, Language: "typescript",
			FilePatterns: []string{}, PackagePrefixes: []string{},
			CustomMappings: make(map[string]string),
		},
		Target:   TargetConfig{OutputDir: "output", FileSuffix: "_converted"},
		Analysis: AnalysisConfig{MaxCallDepth: 5, MaxFiles: 50, EnableDataflow: true, EnableIntent: true},
		CustomMappings: make(map[string]string),
		Coco: CocoConfig{MaxTurns: 10, AllowedTools: []string{}, Timeout: "30s", OutputFormat: "json"},
		Execution:   ExecutionConfig{RetryLimit: 3, MaxJobs: 4},
		MigrationDoc: "",
		TSExtractor: TSExtractorConfig{NodePath: "node", ScriptPath: "scripts/dist/ts-ast-extractor.js", Timeout: 30},
		Fingerprint: FingerprintConfig{CustomMappings: make(map[string]string)},
		SourceDir: ".", Workers: 4,
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing YAML config: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing JSON config: %w", err)
		}
	default:
		if err := yaml.Unmarshal(data, cfg); err != nil {
			if err2 := json.Unmarshal(data, cfg); err2 != nil {
				return nil, fmt.Errorf("unable to parse config as YAML (%v) or JSON (%v)", err, err2)
			}
		}
	}
	cfg.configPath = path
	return cfg, nil
}

func (c *Config) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}
	ext := filepath.Ext(path)
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, c); err != nil {
			return fmt.Errorf("parsing YAML config: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, c); err != nil {
			return fmt.Errorf("parsing JSON config: %w", err)
		}
	default:
		if err := yaml.Unmarshal(data, c); err != nil {
			if err2 := json.Unmarshal(data, c); err2 != nil {
				return fmt.Errorf("unable to parse config as YAML (%v) or JSON (%v)", err, err2)
			}
		}
	}
	c.configPath = path
	return nil
}

func (c *Config) Reset() {
	def := DefaultConfig()
	*c = *def
}

func (c *Config) ToMap() map[string]interface{} {
	keys := c.GetAllKeys()
	m := make(map[string]interface{}, len(keys))
	for _, k := range keys {
		v, err := c.Get(k)
		if err != nil {
			continue
		}
		m[k] = v
	}
	return m
}
