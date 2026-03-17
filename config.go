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
const currentConfigVersion = 2

type BuildConfig struct {
	Context    string                 `json:"context"`
	Dockerfile string                 `json:"dockerfile"`
	Tag        string                 `json:"tag"`
	Env        map[string]string      `json:"env"`
	Args       map[string]string      `json:"args"`
	Secrets    map[string]BuildSecret `json:"secrets"`
	SSH        []string               `json:"ssh"`
	BuildKit   *bool                  `json:"buildkit"`
	CacheFrom  []string               `json:"cache_from"`
	CacheTo    []string               `json:"cache_to"`
	Target     string                 `json:"target"`
	Platform   string                 `json:"platform"`
	NoCache    *bool                  `json:"no_cache"`
	Pull       *bool                  `json:"pull"`
	ExtraArgs  string                 `json:"extra_args"`
	LegacyArgs string                 `json:"-"`
}

type BuildSecret struct {
	Env  string `json:"env"`
	File string `json:"file"`
}

type RunConfig struct {
	Env          map[string]string `json:"env"`
	EnvFile      string            `json:"env_file"`
	Args         string            `json:"args"`
	AutoPort     *bool             `json:"auto_port"`
	Name         string            `json:"name"`
	ZeroDowntime *bool             `json:"zero_downtime,omitempty"`
	Readiness    string            `json:"readiness,omitempty"`
}

type Config struct {
	SchemaVersion int         `json:"schema_version,omitempty"`
	Image         string      `json:"image"`
	EnvFile       string      `json:"env_file,omitempty"`
	AutoPort      *bool       `json:"auto_port,omitempty"`
	RunArgs       string      `json:"run_args,omitempty"`
	Run           RunConfig   `json:"run,omitempty"`
	Build         BuildConfig `json:"build,omitempty"`
}

type BuildOverrides struct {
	Context  string
	File     string
	Tag      string
	Args     string
	Target   string
	Platform string
	NoCache  *bool
	Pull     *bool
	BuildKit *bool
}

func buildRunTrail(cfg Config) string {
	trail := strings.TrimSpace(runArgs(cfg))
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

	cfg = normalizeConfig(cfg)

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
		SchemaVersion: currentConfigVersion,
		Image:         "my-image",
		Run: RunConfig{
			EnvFile:      envFile,
			AutoPort:     &defaultAutoPort,
			Args:         "--rm",
			Name:         "my-app",
			ZeroDowntime: boolPtr(false),
			Readiness:    "healthcheck",
		},
		Build: BuildConfig{
			Context:    ".",
			Dockerfile: "Dockerfile",
			Tag:        "my-image",
			BuildKit:   boolPtr(true),
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

func (c *BuildConfig) UnmarshalJSON(data []byte) error {
	type buildAlias BuildConfig

	aux := struct {
		Args json.RawMessage `json:"args"`
		*buildAlias
	}{
		buildAlias: (*buildAlias)(c),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	rawArgs := strings.TrimSpace(string(aux.Args))
	if rawArgs == "" || rawArgs == "null" {
		return nil
	}

	switch rawArgs[0] {
	case '"':
		return json.Unmarshal(aux.Args, &c.LegacyArgs)
	case '{':
		return json.Unmarshal(aux.Args, &c.Args)
	default:
		return fmt.Errorf("build.args must be an object or string")
	}
}

func runArgs(cfg Config) string {
	if strings.TrimSpace(cfg.Run.Args) != "" {
		return cfg.Run.Args
	}
	return cfg.RunArgs
}

func runEnvFile(cfg Config) string {
	if strings.TrimSpace(cfg.Run.EnvFile) != "" {
		return cfg.Run.EnvFile
	}
	return cfg.EnvFile
}

func runAutoPort(cfg Config) *bool {
	if cfg.Run.AutoPort != nil {
		return cfg.Run.AutoPort
	}
	return cfg.AutoPort
}

func boolPtr(v bool) *bool {
	return &v
}

func normalizeConfig(cfg Config) Config {
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = currentConfigVersion
	}

	if strings.TrimSpace(cfg.Run.EnvFile) == "" && strings.TrimSpace(cfg.EnvFile) != "" {
		cfg.Run.EnvFile = cfg.EnvFile
	}
	if cfg.Run.AutoPort == nil && cfg.AutoPort != nil {
		cfg.Run.AutoPort = cfg.AutoPort
	}
	if strings.TrimSpace(cfg.Run.Args) == "" && strings.TrimSpace(cfg.RunArgs) != "" {
		cfg.Run.Args = cfg.RunArgs
	}

	if strings.TrimSpace(cfg.Build.ExtraArgs) == "" && strings.TrimSpace(cfg.Build.LegacyArgs) != "" {
		cfg.Build.ExtraArgs = cfg.Build.LegacyArgs
	}
	if cfg.Build.BuildKit == nil {
		cfg.Build.BuildKit = boolPtr(true)
	}
	if cfg.Run.ZeroDowntime == nil {
		cfg.Run.ZeroDowntime = boolPtr(false)
	}
	if strings.TrimSpace(cfg.Run.Readiness) == "" {
		cfg.Run.Readiness = "healthcheck"
	}

	cfg.EnvFile = ""
	cfg.AutoPort = nil
	cfg.RunArgs = ""
	cfg.Build.LegacyArgs = ""

	return cfg
}

func validateConfig(cfg Config) error {
	for name, secret := range cfg.Build.Secrets {
		hasEnv := strings.TrimSpace(secret.Env) != ""
		hasFile := strings.TrimSpace(secret.File) != ""
		if hasEnv == hasFile {
			return fmt.Errorf("build.secrets.%s must set exactly one of env or file", name)
		}
	}
	return nil
}

func marshalConfig(cfg Config) ([]byte, error) {
	cfg = normalizeConfig(cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
