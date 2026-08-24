package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type config struct {
	AllowedUsers       []int64            `yaml:"allowed_users"`
	SessionTTLHours    int                `yaml:"session_ttl_hours"`
	SSH                sshConfig          `yaml:"ssh"`
	ConsolePermissions map[string][]int64 `yaml:"console_permissions"`
}

type sshConfig struct {
	MachineID         string   `yaml:"machine_id"`
	Host              string   `yaml:"host"`
	Port              int      `yaml:"port"`
	User              string   `yaml:"user"`
	PrivateKeyPath    string   `yaml:"private_key_path"`
	AgentSocket       string   `yaml:"agent_socket"`
	KnownHostsPath    string   `yaml:"known_hosts_path"`
	HostKeyAlgorithms []string `yaml:"host_key_algorithms"`
}

func loadConfig(path string) (config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (cfg config) allowedUsers() map[int64]struct{} {
	allowed := make(map[int64]struct{}, len(cfg.AllowedUsers))
	for _, id := range cfg.AllowedUsers {
		allowed[id] = struct{}{}
	}
	return allowed
}

func (cfg config) consolePermissions() map[string]map[int64]struct{} {
	permissions := make(map[string]map[int64]struct{}, len(cfg.ConsolePermissions))
	for machineID, userIDs := range cfg.ConsolePermissions {
		users := make(map[int64]struct{}, len(userIDs))
		for _, userID := range userIDs {
			users[userID] = struct{}{}
		}
		permissions[machineID] = users
	}
	return permissions
}
