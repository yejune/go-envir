package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type EnvirConfig struct {
	Servers map[string]Server `yaml:"servers"`
	Tasks   map[string]Task   `yaml:"tasks"`
}

type Server struct {
	Host string `yaml:"host"`
	User string `yaml:"user"`
	Port int    `yaml:"port"`
	Key  string `yaml:"key"` // SSH key path
}

type Task struct {
	Description string   `yaml:"description"`
	On          []string `yaml:"on"`      // Server names
	Scripts     []Script `yaml:"scripts"` // List of scripts
}

type Script struct {
	Local  string `yaml:"local"`  // Local command
	Run    string `yaml:"run"`    // Remote command
	Upload string `yaml:"upload"` // Upload file (local:remote format)
}

func Load(path string) (*EnvirConfig, error) {
	// Default path
	if path == "" {
		path = "Envirfile.yaml"
	}

	// Expand ~ in path
	if len(path) > 2 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	var cfg EnvirConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	for name, server := range cfg.Servers {
		if server.Port == 0 {
			server.Port = 22
		}
		if server.User == "" {
			server.User = os.Getenv("USER")
		}
		cfg.Servers[name] = server
	}

	return &cfg, nil
}
