package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Save writes the current configuration to a YAML file.
// If path is provided, it writes to that path and remembers it.
// If no path is provided, it uses the previously loaded path or
// falls back to the default config path.
func (c *Config) Save(path ...string) error {
	target := c.configPath
	if len(path) > 0 && path[0] != "" {
		target = path[0]
	}
	if target == "" {
		target = defaultConfigPath
	}

	dir := filepath.Dir(target)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config to YAML: %w", err)
	}

	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("writing config file %s: %w", target, err)
	}

	c.configPath = target
	return nil
}

// UsePath sets the preferred config file path for subsequent Save calls.
func (c *Config) UsePath(path string) {
	c.configPath = path
}
