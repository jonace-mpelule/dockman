package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const defaultConfigFile = "dockman.json"
const defaultEnvFile = ".env"

type BuildConfig struct {
	Context    string `json:"context"`
	Dockerfile string `json:"dockerfile"`
	Tag        string `json:"tag"`
	Args       string `json:"args"`
}

type Config struct {
	Image    string      `json:"image"`
	EnvFile  string      `json:"env_file"`
	AutoPort *bool       `json:"auto_port"`
	RunArgs  string      `json:"run_args"`
	Build    BuildConfig `json:"build"`
}

type BuildOverrides struct {
	Context string
	File    string
	Tag     string
	Args    string
}

func buildRunTrail(cfg Config) string {
	trail := strings.TrimSpace(cfg.RunArgs)
	image := strings.TrimSpace(cfg.Image)

	if strings.Contains(trail, "{image}") {
		trail = strings.ReplaceAll(trail, "{image}", image)
		return strings.TrimSpace(trail)
	}

	if image == "" {
		return trail
	}
	if trail == "" {
		return image
	}
	return trail + " " + image
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func writeDefaultConfig(path string, profile string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config already exists at %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	defaultAutoPort := true
	envFile := defaultEnvFile
	if profile != "" {
		envFile = fmt.Sprintf(".env.%s", profile)
	}

	cfg := Config{
		Image:    "my-image",
		EnvFile:  envFile,
		AutoPort: &defaultAutoPort,
		RunArgs:  "--rm",
		Build: BuildConfig{
			Context:    ".",
			Dockerfile: "Dockerfile",
			Tag:        "my-image",
			Args:       "",
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	return nil
}

func configPathForProfile(profile string) string {
	if profile == "" {
		return defaultConfigFile
	}
	return fmt.Sprintf("dockman.%s.json", profile)
}
