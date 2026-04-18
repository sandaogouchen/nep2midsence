package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TsConfigPaths struct {
	BaseUrl     string
	ProjectRoot string
	Patterns    []PathPattern
}

type PathPattern struct {
	Prefix  string
	Targets []string
	HasWild bool
	RawKey  string
}

type rawTSConfig struct {
	Extends         string `json:"extends"`
	CompilerOptions struct {
		BaseURL string              `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

type resolvedTSConfig struct {
	baseURL   string
	patterns  []PathPattern
	configDir string
}

func LoadTsConfig(tsconfigPath string) (*TsConfigPaths, error) {
	absPath, err := filepath.Abs(tsconfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve tsconfig path: %w", err)
	}

	resolved, err := loadResolvedTSConfig(absPath, 0)
	if err != nil {
		return nil, err
	}

	return &TsConfigPaths{
		BaseUrl:     resolved.baseURL,
		ProjectRoot: resolved.configDir,
		Patterns:    resolved.patterns,
	}, nil
}

func (t *TsConfigPaths) CanResolve(importPath string) bool {
	if t == nil {
		return false
	}
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return false
	}
	for _, pattern := range t.Patterns {
		if pathPatternMatches(pattern, importPath) {
			return true
		}
	}
	return false
}

func (t *TsConfigPaths) Resolve(importPath string) []string {
	if t == nil {
		return nil
	}
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return nil
	}

	var resolved []string
	seen := make(map[string]struct{})
	for _, pattern := range t.Patterns {
		if !pathPatternMatches(pattern, importPath) {
			continue
		}

		suffix := ""
		if pattern.HasWild {
			suffix = strings.TrimPrefix(importPath, pattern.Prefix)
		}

		for _, target := range pattern.Targets {
			base := target
			if pattern.HasWild {
				base = filepath.Join(target, filepath.FromSlash(suffix))
			}
			for _, candidate := range buildTSImportCandidates(base) {
				if _, ok := seen[candidate]; ok {
					continue
				}
				seen[candidate] = struct{}{}
				resolved = append(resolved, candidate)
			}
		}
	}
	return resolved
}

func loadResolvedTSConfig(tsconfigPath string, depth int) (*resolvedTSConfig, error) {
	if depth > 5 {
		return nil, fmt.Errorf("tsconfig extends depth exceeded for %s", tsconfigPath)
	}

	raw, err := readRawTSConfig(tsconfigPath)
	if err != nil {
		return nil, err
	}

	configDir := filepath.Dir(tsconfigPath)
	result := &resolvedTSConfig{
		baseURL:   raw.CompilerOptions.BaseURL,
		configDir: configDir,
	}

	if strings.TrimSpace(raw.Extends) != "" {
		parentPath, err := resolveTSConfigExtends(configDir, raw.Extends)
		if err == nil {
			parent, loadErr := loadResolvedTSConfig(parentPath, depth+1)
			if loadErr == nil {
				result.patterns = append(result.patterns, parent.patterns...)
				if result.baseURL == "" {
					result.baseURL = parent.baseURL
				}
			}
		}
	}

	if raw.CompilerOptions.BaseURL == "" && result.baseURL == "" {
		result.baseURL = "."
	}

	baseDir := configDir
	if strings.TrimSpace(result.baseURL) != "" {
		baseDir = filepath.Clean(filepath.Join(configDir, filepath.FromSlash(result.baseURL)))
	}

	if len(raw.CompilerOptions.Paths) > 0 {
		result.patterns = append(result.patterns, buildPathPatterns(baseDir, raw.CompilerOptions.Paths)...)
	}

	return result, nil
}

func readRawTSConfig(tsconfigPath string) (*rawTSConfig, error) {
	data, err := os.ReadFile(tsconfigPath)
	if err != nil {
		return nil, fmt.Errorf("read tsconfig: %w", err)
	}
	var raw rawTSConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse tsconfig %s: %w", tsconfigPath, err)
	}
	return &raw, nil
}

func resolveTSConfigExtends(configDir string, extends string) (string, error) {
	extends = strings.TrimSpace(extends)
	if extends == "" {
		return "", fmt.Errorf("empty extends")
	}

	if strings.HasPrefix(extends, ".") || strings.HasPrefix(extends, "/") {
		resolved := filepath.Join(configDir, filepath.FromSlash(extends))
		return ensureTSConfigFile(resolved)
	}

	packagePath := filepath.Join(configDir, "node_modules", filepath.FromSlash(extends))
	return ensureTSConfigFile(packagePath)
}

func ensureTSConfigFile(path string) (string, error) {
	candidates := []string{path}
	if filepath.Ext(path) == "" {
		candidates = append(candidates, path+".json")
		candidates = append(candidates, filepath.Join(path, "tsconfig.json"))
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return filepath.Clean(candidate), nil
		}
	}
	return "", fmt.Errorf("tsconfig extends target not found: %s", path)
}

func buildPathPatterns(baseDir string, paths map[string][]string) []PathPattern {
	var patterns []PathPattern
	for rawKey, targets := range paths {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}

		hasWild := strings.Contains(key, "*")
		prefix := key
		if hasWild {
			prefix = key[:strings.Index(key, "*")]
		}

		pattern := PathPattern{
			Prefix:  prefix,
			HasWild: hasWild,
			RawKey:  key,
		}
		for _, target := range targets {
			target = strings.TrimSpace(target)
			if target == "" {
				continue
			}
			if strings.Contains(target, "*") {
				target = target[:strings.Index(target, "*")]
			}
			target = filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(target)))
			pattern.Targets = append(pattern.Targets, target)
		}
		if len(pattern.Targets) > 0 {
			patterns = append(patterns, pattern)
		}
	}
	return patterns
}

func pathPatternMatches(pattern PathPattern, importPath string) bool {
	if pattern.HasWild {
		return strings.HasPrefix(importPath, pattern.Prefix)
	}
	return importPath == pattern.Prefix
}

func buildTSImportCandidates(base string) []string {
	if ext := filepath.Ext(base); ext != "" {
		return []string{filepath.Clean(base)}
	}

	exts := []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"}
	candidates := make([]string, 0, len(exts)*2)
	for _, ext := range exts {
		candidates = append(candidates, filepath.Clean(base+ext))
	}
	for _, ext := range exts {
		candidates = append(candidates, filepath.Clean(filepath.Join(base, "index"+ext)))
	}
	return candidates
}
