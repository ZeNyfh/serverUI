package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type config struct {
	AllowedUsers []int64 `yaml:"allowed_users"`
}

func loadAllowedUsers(path string) (map[int64]struct{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	allowed := make(map[int64]struct{}, len(cfg.AllowedUsers))
	for _, id := range cfg.AllowedUsers {
		allowed[id] = struct{}{}
	}
	return allowed, nil
}
