package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen      string  `yaml:"listen"`
	ManageHosts bool    `yaml:"manage_hosts"`
	Routes      []Route `yaml:"routes"`
}

type Route struct {
	Host   string `yaml:"host"`
	Target string `yaml:"target"`
}

// loadConfig reads and parses a YAML config file, applying defaults.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Listen == "" {
		cfg.Listen = ":80"
	}
	return &cfg, nil
}

// writeConfig marshals cfg to YAML and writes it atomically to path.
// Atomic = write to a tempfile in the same directory, then rename over the
// final path. On POSIX this is an atomic replace on the same filesystem.
func writeConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config.yaml.*.tmp")
	if err != nil {
		return fmt.Errorf("create tempfile: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close tempfile: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename tempfile: %w", err)
	}
	return nil
}

// ensureConfig makes sure a config file exists at path. If it does not, the
// parent directory is created and a default config is written.
func ensureConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}

	def := &Config{
		Listen:      ":80",
		ManageHosts: true,
		Routes:      []Route{},
	}
	return writeConfig(path, def)
}
