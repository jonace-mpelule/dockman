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
	Image   string `json:"image"`
	EnvFile string `json:"env_file"`
	Build   struct {
		Context    string `json:"context"`
		Dockerfile string `json:"dockerfile"`
		Tag        string `json:"tag"`
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

	if cfg.EnvFile != ".env.dev" {
		t.Fatalf("unexpected env_file: %q", cfg.EnvFile)
	}
	if cfg.Image != "my-image" {
		t.Fatalf("unexpected image: %q", cfg.Image)
	}
	if cfg.Build.Context != "." || cfg.Build.Dockerfile != "Dockerfile" || cfg.Build.Tag != "my-image" {
		t.Fatalf("unexpected build config: %+v", cfg.Build)
	}
}

func TestRunDockerDryRun(t *testing.T) {
	tmp := t.TempDir()
	path := writeTempFile(t, tmp, ".env", "PORT=1234\nFOO=bar\n")

	out := captureStdout(t, func() {
		err := runDocker("my-image", path, true, true)
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
			Args:       "--no-cache",
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
