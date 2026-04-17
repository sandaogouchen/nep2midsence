package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = ".nep2midsence.yaml"

//go:embed default_wrapper_filter.yaml
var builtInWrapperFilterYAML string

type Config struct {
	Source                SourceConfig        `json:"source" yaml:"source"`
	Target                TargetConfig        `json:"target" yaml:"target"`
	Analysis              AnalysisConfig      `json:"analysis" yaml:"analysis"`
	CustomMappings        map[string]string   `json:"custom_mappings" yaml:"custom_mappings"`
	Coco                  CocoConfig          `json:"coco" yaml:"coco"`
	Execution             ExecutionConfig     `json:"execution" yaml:"execution"`
	MigrationDoc          string              `json:"migration_doc" yaml:"migration_doc"`
	TSExtractor           TSExtractorConfig   `json:"ts_extractor" yaml:"ts_extractor"`
	Fingerprint           FingerprintConfig   `json:"fingerprint" yaml:"fingerprint"`
	WrapperFilter         WrapperFilterConfig `json:"wrapper_filter" yaml:"wrapper_filter"`
	KnownInfraRoots       string              `json:"known_infra_roots" yaml:"known_infra_roots"`
	ForceInfraCalls       string              `json:"force_infra_calls" yaml:"force_infra_calls"`
	ForceBusinessCalls    string              `json:"force_business_calls" yaml:"force_business_calls"`
	ForceInfraMethods     string              `json:"force_infra_methods" yaml:"force_infra_methods"`
	ElementLikeProperties string              `json:"element_like_properties" yaml:"element_like_properties"`
	InfraTerminalMethods  string              `json:"infra_terminal_methods" yaml:"infra_terminal_methods"`
	SourceDir             string              `json:"source_dir" yaml:"source_dir"`
	Workers               int                 `json:"workers" yaml:"workers"`
	configPath            string              `json:"-" yaml:"-"`
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
	BaseDir    string `json:"base_dir" yaml:"base_dir"` // Cross-repo target root; empty = same-repo mode
}

// IsCrossRepo reports whether cross-repo migration mode is active.
func (c *Config) IsCrossRepo() bool {
	return strings.TrimSpace(c.Target.BaseDir) != ""
}

type AnalysisConfig struct {
	MaxCallDepth   int  `json:"max_call_depth" yaml:"max_call_depth"`
	MaxFiles       int  `json:"max_files" yaml:"max_files"`
	EnableDataflow bool `json:"enable_dataflow" yaml:"enable_dataflow"`
	EnableIntent   bool `json:"enable_intent" yaml:"enable_intent"`
}

type CocoConfig struct {
	Timeout string `json:"timeout" yaml:"timeout"`
}

type ExecutionConfig struct {
	// Tool selects which external CLI to run for prompt execution.
	// Supported: coco, cc (claude-code), codex.
	Tool       string `json:"tool" yaml:"tool"`
	RetryLimit int    `json:"retry_limit" yaml:"retry_limit"`
	MaxJobs    int    `json:"max_jobs" yaml:"max_jobs"`
}

type TSExtractorConfig struct {
	NodePath   string `json:"node_path" yaml:"node_path"`
	ScriptPath string `json:"script_path" yaml:"script_path"`
	Timeout    int    `json:"timeout" yaml:"timeout"`
}

type FingerprintConfig struct {
	CustomMappings map[string]string `json:"custom_mappings" yaml:"custom_mappings"`
}

type WrapperFilterConfig struct {
	KnownInfraRoots             []string `json:"known_infra_roots" yaml:"known_infra_roots"`
	KnownBusinessNamePatterns   []string `json:"known_business_name_patterns" yaml:"known_business_name_patterns"`
	ForceInfraCallPatterns      []string `json:"force_infra_call_patterns" yaml:"force_infra_call_patterns"`
	ForceBusinessCallPatterns   []string `json:"force_business_call_patterns" yaml:"force_business_call_patterns"`
	ForceInfraMethods           []string `json:"force_infra_methods" yaml:"force_infra_methods"`
	ElementLikePropertyPatterns []string `json:"element_like_property_patterns" yaml:"element_like_property_patterns"`
	InfraTerminalMethods        []string `json:"infra_terminal_methods" yaml:"infra_terminal_methods"`
}

func DefaultConfig() *Config {
	cfg := &Config{
		Source: SourceConfig{
			Dir: ".", Extensions: []string{".ts"}, Pattern: "**/*",
			Exclude: []string{"node_modules", "dist"}, Language: "typescript",
			FilePatterns: []string{}, PackagePrefixes: []string{},
			CustomMappings: make(map[string]string),
		},
		Target:         TargetConfig{OutputDir: "output", FileSuffix: "_converted"},
		Analysis:       AnalysisConfig{MaxCallDepth: 5, MaxFiles: 50, EnableDataflow: true, EnableIntent: true},
		CustomMappings: make(map[string]string),
		Coco:           CocoConfig{Timeout: "30s"},
		Execution:      ExecutionConfig{Tool: "coco", RetryLimit: 3, MaxJobs: 4},
		MigrationDoc:   "",
		TSExtractor:    TSExtractorConfig{NodePath: "node", ScriptPath: "scripts/dist/ts-ast-extractor.js", Timeout: 30},
		Fingerprint:    FingerprintConfig{CustomMappings: make(map[string]string)},
		SourceDir:      ".", Workers: 4,
	}
	cfg.applyBuiltInWrapperFilter()
	cfg.normalizeWrapperFilterAliases()
	return cfg
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
	cfg.normalizeWrapperFilterAliases()
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
	c.normalizeWrapperFilterAliases()
	c.configPath = path
	return nil
}

func (c *Config) Reset() {
	def := DefaultConfig()
	*c = *def
}

func (c *Config) normalizeWrapperFilterAliases() {
	if c == nil {
		return
	}
	mergeStringList(&c.WrapperFilter.KnownInfraRoots, c.KnownInfraRoots)
	mergeStringList(&c.WrapperFilter.ForceInfraCallPatterns, c.ForceInfraCalls)
	mergeStringList(&c.WrapperFilter.ForceBusinessCallPatterns, c.ForceBusinessCalls)
	mergeStringList(&c.WrapperFilter.ForceInfraMethods, c.ForceInfraMethods)
	mergeStringList(&c.WrapperFilter.ElementLikePropertyPatterns, c.ElementLikeProperties)
	mergeStringList(&c.WrapperFilter.InfraTerminalMethods, c.InfraTerminalMethods)

	// 回写为更易读的根级配置字符串，方便 Save 后用户直接编辑。
	c.KnownInfraRoots = strings.Join(c.WrapperFilter.KnownInfraRoots, ", ")
	c.ForceInfraCalls = strings.Join(c.WrapperFilter.ForceInfraCallPatterns, "\n")
	c.ForceBusinessCalls = strings.Join(c.WrapperFilter.ForceBusinessCallPatterns, "\n")
	c.ForceInfraMethods = strings.Join(c.WrapperFilter.ForceInfraMethods, ", ")
	c.ElementLikeProperties = strings.Join(c.WrapperFilter.ElementLikePropertyPatterns, ", ")
	c.InfraTerminalMethods = strings.Join(c.WrapperFilter.InfraTerminalMethods, ", ")
}

func (c *Config) applyBuiltInWrapperFilter() {
	if c == nil || strings.TrimSpace(builtInWrapperFilterYAML) == "" {
		return
	}
	var builtIn struct {
		WrapperFilter WrapperFilterConfig `yaml:"wrapper_filter"`
	}
	if err := yaml.Unmarshal([]byte(builtInWrapperFilterYAML), &builtIn); err != nil {
		return
	}
	mergeSlice(&c.WrapperFilter.KnownInfraRoots, builtIn.WrapperFilter.KnownInfraRoots)
	mergeSlice(&c.WrapperFilter.KnownBusinessNamePatterns, builtIn.WrapperFilter.KnownBusinessNamePatterns)
	mergeSlice(&c.WrapperFilter.ForceInfraCallPatterns, builtIn.WrapperFilter.ForceInfraCallPatterns)
	mergeSlice(&c.WrapperFilter.ForceBusinessCallPatterns, builtIn.WrapperFilter.ForceBusinessCallPatterns)
	mergeSlice(&c.WrapperFilter.ForceInfraMethods, builtIn.WrapperFilter.ForceInfraMethods)
	mergeSlice(&c.WrapperFilter.ElementLikePropertyPatterns, builtIn.WrapperFilter.ElementLikePropertyPatterns)
	mergeSlice(&c.WrapperFilter.InfraTerminalMethods, builtIn.WrapperFilter.InfraTerminalMethods)
}

func mergeSlice(dst *[]string, values []string) {
	if len(values) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(*dst)+len(values))
	out := make([]string, 0, len(*dst)+len(values))
	for _, item := range *dst {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	*dst = out
}

func mergeStringList(dst *[]string, raw string) {
	mergeSlice(dst, splitConfigList(raw))
}

func splitConfigList(raw string) []string {
	replacer := strings.NewReplacer("\r\n", "\n", "；", ";", "，", ",")
	raw = replacer.Replace(raw)
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', '\n':
			return true
		default:
			return false
		}
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
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

// ConfigDir returns the directory containing the configuration file.
// This is used as the project root for storing state and other artifacts,
// ensuring they are never written into user-selected scan directories.
func (c *Config) ConfigDir() string {
	if c.configPath == "" {
		return "."
	}
	dir := filepath.Dir(c.configPath)
	if dir == "" {
		return "."
	}
	return dir
}
