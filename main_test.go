package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testConfig struct {
	SchemaVersion int    `json:"schema_version"`
	Image         string `json:"image"`
	Run           struct {
		EnvFile  string `json:"env_file"`
		Args     string `json:"args"`
		AutoPort *bool  `json:"auto_port"`
	} `json:"run"`
	Build struct {
		Context    string `json:"context"`
		Dockerfile string `json:"dockerfile"`
		Tag        string `json:"tag"`
		BuildKit   *bool  `json:"buildkit"`
		ExtraArgs  string `json:"extra_args"`
	} `json:"build"`
}

func writeTempFile(t *testing.T, dir string, name string, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	_ = w.Close()
	data, _ := io.ReadAll(r)
	_ = r.Close()
	return string(data)
}

func TestParseEnvAdvanced(t *testing.T) {
	tmp := t.TempDir()
	path := writeTempFile(t, tmp, ".env", `# comment
export FOO=bar
BAR="baz qux"
BAZ='quoted'
INLINE=val # comment
HASH=val#notcomment
EMPTY=
INVALIDLINE
`)

	env, err := parseEnv(path)
	if err != nil {
		t.Fatalf("parseEnv: %v", err)
	}

	cases := map[string]string{
		"FOO":    "bar",
		"BAR":    "baz qux",
		"BAZ":    "quoted",
		"INLINE": "val",
		"HASH":   "val#notcomment",
		"EMPTY":  "",
	}

	for key, expected := range cases {
		if got := env[key]; got != expected {
			t.Fatalf("%s: expected %q, got %q", key, expected, got)
		}
	}
}

func TestStripInlineComment(t *testing.T) {
	if got := stripInlineComment("val # comment"); got != "val" {
		t.Fatalf("expected inline comment stripped, got %q", got)
	}
	if got := stripInlineComment("val#notcomment"); got != "val#notcomment" {
		t.Fatalf("expected hash preserved, got %q", got)
	}
	if got := stripInlineComment("val\t#comment"); got != "val" {
		t.Fatalf("expected inline comment stripped with tab, got %q", got)
	}
}

func TestBuildRunTrail(t *testing.T) {
	cfg := Config{Image: "img", RunArgs: "--rm -p 80:80"}
	if got := buildRunTrail(cfg); got != "--rm -p 80:80 img" {
		t.Fatalf("unexpected trail: %q", got)
	}

	cfg = Config{Image: "img", RunArgs: "run {image} --rm"}
	if got := buildRunTrail(cfg); got != "run img --rm" {
		t.Fatalf("unexpected trail with placeholder: %q", got)
	}

	cfg = Config{Image: "img", RunArgs: ""}
	if got := buildRunTrail(cfg); got != "img" {
		t.Fatalf("unexpected trail with image only: %q", got)
	}

	cfg = Config{Image: "", RunArgs: "echo"}
	if got := buildRunTrail(cfg); got != "echo" {
		t.Fatalf("unexpected trail with no image: %q", got)
	}
}

func TestHasPortFlag(t *testing.T) {
	if !hasPortFlag([]string{"-p", "80:80"}) {
		t.Fatalf("expected -p flag detected")
	}
	if !hasPortFlag([]string{"--publish=80:80"}) {
		t.Fatalf("expected --publish detected")
	}
	if !hasPortFlag([]string{"-p80:80"}) {
		t.Fatalf("expected -p prefix detected")
	}
	if hasPortFlag([]string{"--name", "demo"}) {
		t.Fatalf("expected no port flag")
	}
}

func TestExtractGlobalOptions(t *testing.T) {
	options, filtered := extractGlobalOptions([]string{
		"--config=cfg.json",
		"--profile=dev",
		"--dry-run",
		"--help",
		"--version",
		"run",
		"--tc=img",
	})

	if options.ConfigPath != "cfg.json" {
		t.Fatalf("unexpected config path: %q", options.ConfigPath)
	}
	if options.Profile != "dev" {
		t.Fatalf("unexpected profile: %q", options.Profile)
	}
	if !options.DryRun || !options.Help || !options.Version {
		t.Fatalf("expected dry-run/help/version enabled")
	}

	if strings.Join(filtered, " ") != "run --tc=img" {
		t.Fatalf("unexpected filtered args: %v", filtered)
	}
}

func TestConfigPathForProfile(t *testing.T) {
	if got := configPathForProfile(""); got != "dockman.json" {
		t.Fatalf("unexpected default config path: %q", got)
	}
	if got := configPathForProfile("dev"); got != "dockman.dev.json" {
		t.Fatalf("unexpected profile config path: %q", got)
	}
}

func TestWriteDefaultConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "dockman.dev.json")
	if err := writeDefaultConfig(path, "dev"); err != nil {
		t.Fatalf("writeDefaultConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var cfg testConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if cfg.Run.EnvFile != ".env.dev" {
		t.Fatalf("unexpected env_file: %q", cfg.Run.EnvFile)
	}
	if cfg.Image != "my-image" {
		t.Fatalf("unexpected image: %q", cfg.Image)
	}
	if cfg.Build.Context != "." || cfg.Build.Dockerfile != "Dockerfile" || cfg.Build.Tag != "my-image" {
		t.Fatalf("unexpected build config: %+v", cfg.Build)
	}
	if cfg.Build.BuildKit == nil || !*cfg.Build.BuildKit {
		t.Fatalf("expected buildkit enabled by default")
	}
	if cfg.Run.Args != "--rm" {
		t.Fatalf("unexpected run args: %q", cfg.Run.Args)
	}
	if cfg.SchemaVersion != currentConfigVersion {
		t.Fatalf("unexpected schema version: %d", cfg.SchemaVersion)
	}
}

func TestRunDockerDryRun(t *testing.T) {
	tmp := t.TempDir()
	path := writeTempFile(t, tmp, ".env", "PORT=1234\nFOO=bar\n")

	out := captureStdout(t, func() {
		err := runDocker("my-image", path, nil, true, true)
		if err != nil {
			t.Fatalf("runDocker: %v", err)
		}
	})

	if !strings.Contains(out, "docker run") {
		t.Fatalf("expected docker run in output, got %q", out)
	}
	if !strings.Contains(out, "-p 1234:1234") {
		t.Fatalf("expected port mapping in output, got %q", out)
	}
}

func TestBuildDockerDryRun(t *testing.T) {
	cfg := Config{
		Image: "my-image",
		Build: BuildConfig{
			Context:    ".",
			Dockerfile: "Dockerfile",
			Tag:        "my-image",
			LegacyArgs: "--no-cache",
		},
	}

	out := captureStdout(t, func() {
		err := buildDocker(cfg, BuildOverrides{}, true)
		if err != nil {
			t.Fatalf("buildDocker: %v", err)
		}
	})

	if !strings.Contains(out, "docker build") {
		t.Fatalf("expected docker build in output, got %q", out)
	}
	if !strings.Contains(out, "-t my-image") {
		t.Fatalf("expected tag in output, got %q", out)
	}
	if !strings.Contains(out, "-f Dockerfile") {
		t.Fatalf("expected dockerfile in output, got %q", out)
	}
}

func TestBuildConfigLegacyArgsString(t *testing.T) {
	var cfg Config
	data := []byte(`{
	  "build": {
	    "context": ".",
	    "dockerfile": "Dockerfile",
	    "tag": "my-image",
	    "args": "--no-cache --pull"
	  }
	}`)

	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if cfg.Build.LegacyArgs != "--no-cache --pull" {
		t.Fatalf("unexpected legacy args: %q", cfg.Build.LegacyArgs)
	}
}

func TestBuildDockerBuildKitSecretsDryRun(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	cfg := Config{
		Image: "my-image",
		Build: BuildConfig{
			Context:    ".",
			Dockerfile: "Dockerfile",
			Tag:        "my-image",
			Env: map[string]string{
				"GIT_AUTH_TOKEN": "${GITHUB_TOKEN}",
			},
			Args: map[string]string{
				"GITHUB_TOKEN": "${GITHUB_TOKEN}",
			},
			Secrets: map[string]BuildSecret{
				"github_token": {Env: "GITHUB_TOKEN"},
			},
			SSH:       []string{"default"},
			Target:    "builder",
			Platform:  "linux/amd64",
			NoCache:   boolPtr(true),
			Pull:      boolPtr(true),
			CacheFrom: []string{"type=registry,ref=example/app:cache"},
			CacheTo:   []string{"type=inline"},
		},
	}

	out := captureStdout(t, func() {
		err := buildDocker(cfg, BuildOverrides{}, true)
		if err != nil {
			t.Fatalf("buildDocker: %v", err)
		}
	})

	if !strings.Contains(out, "DOCKER_BUILDKIT=1") {
		t.Fatalf("expected buildkit env in output, got %q", out)
	}
	if !strings.Contains(out, "--secret id=github_token,env=GITHUB_TOKEN") {
		t.Fatalf("expected secret flag in output, got %q", out)
	}
	if !strings.Contains(out, "--ssh default") {
		t.Fatalf("expected ssh flag in output, got %q", out)
	}
	if !strings.Contains(out, "--target builder") || !strings.Contains(out, "--platform linux/amd64") {
		t.Fatalf("expected target and platform in output, got %q", out)
	}
	if !strings.Contains(out, "--cache-from type=registry,ref=example/app:cache") || !strings.Contains(out, "--cache-to type=inline") {
		t.Fatalf("expected cache flags in output, got %q", out)
	}
	if !strings.Contains(out, "GIT_AUTH_TOKEN=<redacted>") || !strings.Contains(out, "GITHUB_TOKEN=<redacted>") {
		t.Fatalf("expected redacted values in output, got %q", out)
	}
	if strings.Contains(out, "secret-token") {
		t.Fatalf("expected secret value redacted, got %q", out)
	}
}

func TestBuildDockerMissingInterpolatedVariable(t *testing.T) {
	cfg := Config{
		Build: BuildConfig{
			Args: map[string]string{
				"NPM_TOKEN": "${MISSING_TOKEN}",
			},
		},
	}

	err := buildDocker(cfg, BuildOverrides{}, true)
	if err == nil {
		t.Fatalf("expected missing variable error")
	}
	if !strings.Contains(err.Error(), "build.args.NPM_TOKEN") || !strings.Contains(err.Error(), "MISSING_TOKEN") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeConfigMovesLegacyFields(t *testing.T) {
	autoPort := true
	cfg := normalizeConfig(Config{
		Image:    "app",
		EnvFile:  ".env",
		AutoPort: &autoPort,
		RunArgs:  "--rm",
		Build: BuildConfig{
			LegacyArgs: "--pull",
		},
	})

	if cfg.SchemaVersion != currentConfigVersion {
		t.Fatalf("unexpected schema version: %d", cfg.SchemaVersion)
	}
	if cfg.Run.EnvFile != ".env" {
		t.Fatalf("unexpected run env file: %q", cfg.Run.EnvFile)
	}
	if cfg.Run.AutoPort == nil || !*cfg.Run.AutoPort {
		t.Fatalf("expected run auto port true")
	}
	if cfg.Run.Args != "--rm" {
		t.Fatalf("unexpected run args: %q", cfg.Run.Args)
	}
	if cfg.Build.ExtraArgs != "--pull" {
		t.Fatalf("unexpected build extra args: %q", cfg.Build.ExtraArgs)
	}
	if cfg.Build.BuildKit == nil || !*cfg.Build.BuildKit {
		t.Fatalf("expected buildkit enabled")
	}
	if cfg.EnvFile != "" || cfg.AutoPort != nil || cfg.RunArgs != "" {
		t.Fatalf("expected legacy fields cleared: %+v", cfg)
	}
}

func TestDoctorConfigUpdatesLegacyFile(t *testing.T) {
	tmp := t.TempDir()
	path := writeTempFile(t, tmp, "dockman.json", `{
  "image": "legacy-image",
  "env_file": ".env.dev",
  "auto_port": true,
  "run_args": "--rm",
  "build": {
    "context": ".",
    "dockerfile": "Dockerfile",
    "tag": "legacy-image",
    "args": "--pull"
  }
}
`)

	if err := doctorConfig(path, false); err != nil {
		t.Fatalf("doctorConfig: %v", err)
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !strings.Contains(string(backup), `"env_file": ".env.dev"`) {
		t.Fatalf("expected backup to contain original config")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}

	var cfg testConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal updated config: %v", err)
	}

	if cfg.SchemaVersion != currentConfigVersion {
		t.Fatalf("unexpected schema version: %d", cfg.SchemaVersion)
	}
	if cfg.Run.EnvFile != ".env.dev" || cfg.Run.Args != "--rm" {
		t.Fatalf("unexpected run config: %+v", cfg.Run)
	}
	if cfg.Run.AutoPort == nil || !*cfg.Run.AutoPort {
		t.Fatalf("expected auto_port migrated")
	}
	if cfg.Build.ExtraArgs != "--pull" {
		t.Fatalf("expected legacy build args migrated to extra_args, got %q", cfg.Build.ExtraArgs)
	}
	if strings.Contains(string(data), `"env_file": ".env.dev"`) && !strings.Contains(string(data), `"run"`) {
		t.Fatalf("expected canonical run config in updated file")
	}
}

func TestDoctorConfigDryRunLeavesFileUntouched(t *testing.T) {
	tmp := t.TempDir()
	original := `{"image":"legacy","env_file":".env"}`
	path := writeTempFile(t, tmp, "dockman.json", original)

	if err := doctorConfig(path, true); err != nil {
		t.Fatalf("doctorConfig dry run: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(data) != original {
		t.Fatalf("expected dry run to leave file untouched, got %q", string(data))
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("expected no backup on dry run, err=%v", err)
	}
}

func TestDoctorConfigRejectsInvalidSecrets(t *testing.T) {
	tmp := t.TempDir()
	path := writeTempFile(t, tmp, "dockman.json", `{
  "image": "bad",
  "build": {
    "secrets": {
      "token": {
        "env": "TOKEN",
        "file": ".token"
      }
    }
  }
}
`)

	err := doctorConfig(path, false)
	if err == nil {
		t.Fatalf("expected invalid secret error")
	}
	if !strings.Contains(err.Error(), "build.secrets.token") {
		t.Fatalf("unexpected error: %v", err)
	}
}
