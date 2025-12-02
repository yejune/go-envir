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
	Log     LogConfig         `yaml:"log"`
}

type LogConfig struct {
	Enabled bool   `yaml:"enabled"` // Enable logging to file
	Path    string `yaml:"path"`    // Log file path (default: ./envir.log)
}

// Server can have single host or multiple hosts
type Server struct {
	Host      string   `yaml:"host"`  // Single host
	HostsYAML []string `yaml:"hosts"` // Multiple hosts (YAML key)
	User      string   `yaml:"user"`
	Port      int      `yaml:"port"`
	Key       string   `yaml:"key"`   // SSH key path
	Hosts     []string `yaml:"-"`     // Expanded hosts (internal use)
}

type Task struct {
	Description string   `yaml:"description"`
	On          []string `yaml:"on"`       // Server names
	Parallel    bool     `yaml:"parallel"` // Run on servers in parallel
	Scripts     []Script `yaml:"scripts"`  // List of scripts
}

type Script struct {
	Local string `yaml:"local"` // Local command
	Run   string `yaml:"run"`   // Remote command
	Sync  string `yaml:"sync"`  // Sync upload (changed files only, checksum comparison)
	Tar   string `yaml:"tar"`   // Tar upload (compress, upload, extract - atomic)
	Scp   string `yaml:"scp"`   // SCP upload (direct transfer, no checksum)
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

	// Expand servers with multiple hosts
	expandedServers := make(map[string]Server)
	for name, server := range cfg.Servers {
		// Set defaults
		if server.Port == 0 {
			server.Port = 22
		}
		if server.User == "" {
			server.User = os.Getenv("USER")
		}

		// Determine hosts (prefer HostsYAML over Host)
		var hosts []string
		if len(server.HostsYAML) > 0 {
			hosts = server.HostsYAML
		} else if server.Host != "" {
			hosts = []string{server.Host}
		}

		if len(hosts) == 1 {
			// Single host - keep as is
			server.Hosts = hosts
			expandedServers[name] = server
		} else if len(hosts) > 1 {
			// Multiple hosts - expand to separate servers
			for i, host := range hosts {
				expandedServer := Server{
					Host:  host,
					Hosts: []string{host},
					User:  server.User,
					Port:  server.Port,
					Key:   server.Key,
				}
				// Name format: web[0], web[1], etc.
				expandedName := fmt.Sprintf("%s[%d]", name, i)
				expandedServers[expandedName] = expandedServer
			}
			// Also keep a reference for task resolution
			server.Hosts = hosts
			expandedServers[name] = server
		}
	}
	cfg.Servers = expandedServers

	return &cfg, nil
}

// GetExpandedServers returns server names to run on
// If server has multiple hosts, returns expanded names (web[0], web[1], etc.)
func (cfg *EnvirConfig) GetExpandedServers(names []string) []string {
	var result []string
	for _, name := range names {
		if server, ok := cfg.Servers[name]; ok {
			if len(server.Hosts) > 1 {
				// Multiple hosts - expand
				for i := range server.Hosts {
					result = append(result, fmt.Sprintf("%s[%d]", name, i))
				}
			} else {
				result = append(result, name)
			}
		} else {
			result = append(result, name)
		}
	}
	return result
}
