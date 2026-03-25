package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type testConfig struct {
	SchemaVersion int    `json:"schema_version"`
	Image         string `json:"image"`
	Run           struct {
		EnvFile      string `json:"env_file"`
		Args         string `json:"args"`
		AutoPort     *bool  `json:"auto_port"`
		Name         string `json:"name"`
		ZeroDowntime *bool  `json:"zero_downtime"`
		Readiness    string `json:"readiness"`
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
		"--allow-no-env",
		"--plain",
		"-y",
		"--update",
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
	if !options.AllowNoEnv {
		t.Fatalf("expected allow-no-env enabled")
	}
	if !options.InitPlain || !options.InitYes {
		t.Fatalf("expected init flags enabled")
	}
	if !options.Update {
		t.Fatalf("expected update enabled")
	}

	if strings.Join(filtered, " ") != "run --tc=img" {
		t.Fatalf("unexpected filtered args: %v", filtered)
	}
}

func TestSelfUpdateDryRun(t *testing.T) {
	oldRunner := updateCommandRunner
	defer func() {
		updateCommandRunner = oldRunner
	}()

	called := false
	updateCommandRunner = func(dryRun bool) error {
		called = true
		if !dryRun {
			t.Fatalf("expected dry-run update")
		}
		return nil
	}

	if err := selfUpdate(true); err != nil {
		t.Fatalf("selfUpdate: %v", err)
	}
	if !called {
		t.Fatalf("expected update runner called")
	}
}

func TestUpdateCommandRunnerDryRunOutput(t *testing.T) {
	out := captureStdout(t, func() {
		if err := updateCommandRunner(true); err != nil {
			t.Fatalf("updateCommandRunner: %v", err)
		}
	})

	if !strings.Contains(out, "NONINTERACTIVE=1 sh") {
		t.Fatalf("expected non-interactive installer invocation, got %q", out)
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
	t.Chdir(tmp)
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
	expectedProjectName := sanitizeProjectName(filepath.Base(tmp))
	if cfg.Image != expectedProjectName {
		t.Fatalf("unexpected image: %q", cfg.Image)
	}
	if cfg.Build.Context != "." || cfg.Build.Dockerfile != "Dockerfile" || cfg.Build.Tag != expectedProjectName {
		t.Fatalf("unexpected build config: %+v", cfg.Build)
	}
	if cfg.Build.BuildKit == nil || !*cfg.Build.BuildKit {
		t.Fatalf("expected buildkit enabled by default")
	}
	if cfg.Run.Args != "--rm" {
		t.Fatalf("unexpected run args: %q", cfg.Run.Args)
	}
	if cfg.Run.Name != expectedProjectName {
		t.Fatalf("unexpected managed name: %q", cfg.Run.Name)
	}
	if cfg.Run.ZeroDowntime == nil || *cfg.Run.ZeroDowntime {
		t.Fatalf("expected zero downtime disabled by default")
	}
	if cfg.Run.Readiness != "healthcheck" {
		t.Fatalf("unexpected readiness: %q", cfg.Run.Readiness)
	}
	if cfg.SchemaVersion != currentConfigVersion {
		t.Fatalf("unexpected schema version: %d", cfg.SchemaVersion)
	}
}

func TestDefaultConfigUsesProfileDefaults(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)
	cfg := defaultConfig("prod")

	if cfg.Run.EnvFile != ".env.prod" {
		t.Fatalf("unexpected env file: %q", cfg.Run.EnvFile)
	}
	expectedProjectName := sanitizeProjectName(filepath.Base(tmp))
	if cfg.Image != expectedProjectName || cfg.Run.Name != expectedProjectName || cfg.Build.Tag != expectedProjectName {
		t.Fatalf("expected project-derived defaults, got image=%q name=%q tag=%q", cfg.Image, cfg.Run.Name, cfg.Build.Tag)
	}
	if cfg.Build.BuildKit == nil || !*cfg.Build.BuildKit {
		t.Fatalf("expected buildkit enabled")
	}
}

func TestSanitizeProjectName(t *testing.T) {
	if got := sanitizeProjectName("My Cool_App v2"); got != "my-cool_app-v2" {
		t.Fatalf("unexpected sanitized project name: %q", got)
	}
	if got := sanitizeProjectName("..."); got != "app" {
		t.Fatalf("expected fallback app name, got %q", got)
	}
}

func TestShouldUseInteractiveInitTTYSelection(t *testing.T) {
	oldChecker := terminalChecker
	defer func() {
		terminalChecker = oldChecker
	}()

	terminalChecker = func(f *os.File) bool {
		return f == os.Stdin || f == os.Stdout
	}

	if !shouldUseInteractiveInit(GlobalOptions{}) {
		t.Fatalf("expected interactive init when stdin/stdout are terminals")
	}
	if shouldUseInteractiveInit(GlobalOptions{InitPlain: true}) {
		t.Fatalf("expected --plain to disable interactive init")
	}
	if shouldUseInteractiveInit(GlobalOptions{InitYes: true}) {
		t.Fatalf("expected --yes to disable interactive init")
	}
}

func TestRunInitUsesInteractiveRunnerOnTTY(t *testing.T) {
	oldChecker := terminalChecker
	oldRunner := interactiveInitRunner
	defer func() {
		terminalChecker = oldChecker
		interactiveInitRunner = oldRunner
	}()

	terminalChecker = func(*os.File) bool { return true }
	interactiveInitRunner = func(path string, profile string) (Config, bool, error) {
		cfg := defaultConfig(profile)
		cfg.Image = "interactive-image"
		return cfg, false, nil
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "dockman.json")
	if err := runInit(path, "", GlobalOptions{}); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `"image": "interactive-image"`) {
		t.Fatalf("expected interactive config written, got %q", string(data))
	}
}

func TestRunInitPlainRefusesOverwrite(t *testing.T) {
	oldChecker := terminalChecker
	defer func() {
		terminalChecker = oldChecker
	}()
	terminalChecker = func(*os.File) bool { return true }

	tmp := t.TempDir()
	path := writeTempFile(t, tmp, "dockman.json", "{}")

	err := runInit(path, "", GlobalOptions{InitPlain: true})
	if err == nil {
		t.Fatalf("expected overwrite refusal")
	}
	if !strings.Contains(err.Error(), "config already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInitYesWritesDefaultConfig(t *testing.T) {
	oldChecker := terminalChecker
	defer func() {
		terminalChecker = oldChecker
	}()
	terminalChecker = func(*os.File) bool { return true }

	tmp := t.TempDir()
	path := filepath.Join(tmp, "dockman.dev.json")
	if err := runInit(path, "dev", GlobalOptions{InitYes: true}); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `"env_file": ".env.dev"`) {
		t.Fatalf("expected profile env file in config, got %q", string(data))
	}
}

func TestPrintHelpIncludesInitModes(t *testing.T) {
	out := captureStdout(t, printHelp)

	if !strings.Contains(out, "dockman init [--plain] [--yes|-y]") {
		t.Fatalf("expected init flags in help, got %q", out)
	}
	if !strings.Contains(out, "--yes (or -y)") {
		t.Fatalf("expected -y alias in help, got %q", out)
	}
	if !strings.Contains(out, "interactive wizard") {
		t.Fatalf("expected interactive wizard help text, got %q", out)
	}
}

func TestPrintVersionIsBranded(t *testing.T) {
	oldVersion := version
	version = "v9.9.9"
	defer func() {
		version = oldVersion
	}()

	out := captureStdout(t, printVersion)

	if !strings.Contains(out, "Dockman") {
		t.Fatalf("expected branded name in version output, got %q", out)
	}
	if !strings.Contains(out, "\033[38;5;214m  ____             __") {
		t.Fatalf("expected orange ascii banner in version output, got %q", out)
	}
	if !strings.Contains(out, "Version  v9.9.9") {
		t.Fatalf("expected version in branded output, got %q", out)
	}
	if !strings.Contains(out, "____") {
		t.Fatalf("expected ascii branding in version output, got %q", out)
	}
}

func TestPrintInitSuccessIsBranded(t *testing.T) {
	out := captureStdout(t, func() {
		printInitSuccess("dockman.json", "dev")
	})

	if !strings.Contains(out, "\033[38;5;214m  ____             __") {
		t.Fatalf("expected orange ascii banner in init success output, got %q", out)
	}
	if !strings.Contains(out, "Dockman init complete") {
		t.Fatalf("expected branded init completion text, got %q", out)
	}
	if !strings.Contains(out, "Config    dockman.json") {
		t.Fatalf("expected config path in init completion output, got %q", out)
	}
	if !strings.Contains(out, "Profile   dev") {
		t.Fatalf("expected profile in init completion output, got %q", out)
	}
	if !strings.Contains(out, "run dockman to use the config-driven flow") {
		t.Fatalf("expected next-step guidance in init completion output, got %q", out)
	}
}

func TestRunDockerDryRun(t *testing.T) {
	tmp := t.TempDir()
	path := writeTempFile(t, tmp, ".env", "PORT=1234\nFOO=bar\n")

	out := captureStdout(t, func() {
		err := runDocker("my-image", path, nil, true, false, true)
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

func TestRunDockerAllowNoEnv(t *testing.T) {
	out := captureStdout(t, func() {
		err := runDocker("my-image", filepath.Join(t.TempDir(), ".env.missing"), map[string]string{"FOO": "bar"}, true, true, true)
		if err != nil {
			t.Fatalf("runDocker: %v", err)
		}
	})

	if !strings.Contains(out, "FOO=<redacted>") {
		t.Fatalf("expected inline env in output, got %q", out)
	}
	if strings.Contains(out, "error opening env file") {
		t.Fatalf("expected missing env tolerated, got %q", out)
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
	if cfg.Run.ZeroDowntime == nil || *cfg.Run.ZeroDowntime {
		t.Fatalf("expected zero downtime defaulted false")
	}
	if cfg.Run.Readiness != "healthcheck" {
		t.Fatalf("expected readiness defaulted")
	}
	if cfg.EnvFile != "" || cfg.AutoPort != nil || cfg.RunArgs != "" {
		t.Fatalf("expected legacy fields cleared: %+v", cfg)
	}
}

func TestManagedStartDryRun(t *testing.T) {
	cfg := Config{
		Image: "my-image",
		Run: RunConfig{
			Name:     "app",
			Args:     "--rm",
			AutoPort: boolPtr(true),
		},
	}

	out := captureStdout(t, func() {
		err := runManagedLifecycle("start", cfg, managedRuntimeOptions{DryRun: true, AllowNoEnv: true})
		if err != nil {
			t.Fatalf("managed start dry run: %v", err)
		}
	})

	if !strings.Contains(out, "docker run -d --name app") {
		t.Fatalf("expected managed run command, got %q", out)
	}
	if !strings.Contains(out, "adoptable container names app") {
		t.Fatalf("expected adoptable names in output, got %q", out)
	}
}

func TestManagedRestartZeroDowntimeDryRun(t *testing.T) {
	cfg := Config{
		Image: "my-image",
		Run: RunConfig{
			Name:         "app",
			Args:         "--publish=127.0.0.1:8080:8080 --rm",
			AutoPort:     boolPtr(true),
			ZeroDowntime: boolPtr(true),
			Readiness:    "healthcheck",
		},
	}

	out := captureStdout(t, func() {
		err := runManagedLifecycle("restart", cfg, managedRuntimeOptions{DryRun: true, AllowNoEnv: true})
		if err != nil {
			t.Fatalf("managed restart dry run: %v", err)
		}
	})

	if !strings.Contains(out, "docker network create app-net") {
		t.Fatalf("expected network create in output, got %q", out)
	}
	if !strings.Contains(out, "docker image inspect --format") {
		t.Fatalf("expected healthcheck inspect in output, got %q", out)
	}
	if !strings.Contains(out, "docker exec app-proxy nginx -s reload") {
		t.Fatalf("expected proxy reload in output, got %q", out)
	}
	if !strings.Contains(out, "127.0.0.1:8080:8080") {
		t.Fatalf("expected publish spec preserved, got %q", out)
	}
}

func TestResolveManagedRuntimeExtractsLegacyName(t *testing.T) {
	cfg := Config{
		Image: "my-image",
		Run: RunConfig{
			Name:     "managed-app",
			Args:     "--name legacy-app --rm",
			AutoPort: boolPtr(true),
		},
	}

	rt, err := resolveManagedRuntime(cfg, managedRuntimeOptions{AllowNoEnv: true})
	if err != nil {
		t.Fatalf("resolveManagedRuntime: %v", err)
	}

	if rt.Name != "managed-app" {
		t.Fatalf("unexpected managed name: %q", rt.Name)
	}
	if rt.LegacyName != "legacy-app" {
		t.Fatalf("unexpected legacy name: %q", rt.LegacyName)
	}
}

func TestManagedRestartDryRunShowsLegacyCleanup(t *testing.T) {
	cfg := Config{
		Image: "my-image",
		Run: RunConfig{
			Name:     "managed-app",
			Args:     "--name legacy-app --rm",
			AutoPort: boolPtr(true),
			Env: map[string]string{
				"PORT": "8080",
			},
		},
	}

	out := captureStdout(t, func() {
		err := runManagedLifecycle("restart", cfg, managedRuntimeOptions{DryRun: true, AllowNoEnv: true})
		if err != nil {
			t.Fatalf("managed restart dry run: %v", err)
		}
	})

	if !strings.Contains(out, "adoptable container names managed-app, legacy-app") {
		t.Fatalf("expected adoptable names in output, got %q", out)
	}
	if !strings.Contains(out, "docker rm -f managed-app") {
		t.Fatalf("expected managed-name cleanup in output, got %q", out)
	}
	if !strings.Contains(out, "docker rm -f legacy-app") {
		t.Fatalf("expected legacy-name cleanup in output, got %q", out)
	}
	if !strings.Contains(out, "docker ps --filter publish=8080 --format {{.Names}}") {
		t.Fatalf("expected port conflict probe in output, got %q", out)
	}
	if !strings.Contains(out, "docker run -d --name managed-app") {
		t.Fatalf("expected restart to recreate managed name, got %q", out)
	}
}

func TestHostPortFromPublishSpec(t *testing.T) {
	hostPort, err := hostPortFromPublishSpec("127.0.0.1:8080:3000")
	if err != nil {
		t.Fatalf("hostPortFromPublishSpec: %v", err)
	}
	if hostPort != "8080" {
		t.Fatalf("unexpected host port: %q", hostPort)
	}
}

func TestAdoptableNamesDeduplicatesManagedAndLegacy(t *testing.T) {
	rt := managedRuntime{
		Name:       "app",
		LegacyName: "app",
	}

	names := adoptableNames(rt)
	if len(names) != 1 || names[0] != "app" {
		t.Fatalf("unexpected adoptable names: %#v", names)
	}
}

func TestManagedUpgradeDryRunUsesOverrideImage(t *testing.T) {
	cfg := Config{
		Image: "my-image:old",
		Run: RunConfig{
			Name:         "app",
			AutoPort:     boolPtr(true),
			ZeroDowntime: boolPtr(true),
			Readiness:    "healthcheck",
			Env: map[string]string{
				"PORT": "8080",
			},
		},
	}

	out := captureStdout(t, func() {
		err := runManagedLifecycle("upgrade", cfg, managedRuntimeOptions{
			DryRun:     true,
			AllowNoEnv: true,
			Image:      "ghcr.io/example/app:new",
		})
		if err != nil {
			t.Fatalf("managed upgrade dry run: %v", err)
		}
	})

	if !strings.Contains(out, "docker pull ghcr.io/example/app:new") {
		t.Fatalf("expected image pull in output, got %q", out)
	}
	if !strings.Contains(out, "ghcr.io/example/app:new") {
		t.Fatalf("expected override image in output, got %q", out)
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

func TestInstallScriptSmokeNonInteractive(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	installDir := filepath.Join(tmp, "install")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	writeTempFile(t, binDir, "uname", "#!/bin/sh\nif [ \"$1\" = \"-m\" ]; then\n  echo arm64\nelse\n  echo Darwin\nfi\n")
	writeTempFile(t, binDir, "curl", "#!/bin/sh\nout=\"\"\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = \"-o\" ]; then\n    out=\"$2\"\n    shift 2\n    continue\n  fi\n  shift\ndone\nprintf '#!/bin/sh\\necho dockman\\n' > \"$out\"\n")
	for _, name := range []string{"uname", "curl"} {
		if err := os.Chmod(filepath.Join(binDir, name), 0o755); err != nil {
			t.Fatalf("chmod %s: %v", name, err)
		}
	}

	cmd := exec.Command("sh", "scripts/install.sh")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"NONINTERACTIVE=1",
		"VERSION=v9.9.9",
		"INSTALL_DIR="+installDir,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, string(out))
	}

	output := string(out)
	if !strings.Contains(output, "Platform     darwin/arm64") {
		t.Fatalf("expected normalized platform in output, got %q", output)
	}
	if !strings.Contains(output, "Install to   "+installDir+"/dockman") {
		t.Fatalf("expected install dir in output, got %q", output)
	}
	if !strings.Contains(output, "Installed dockman v9.9.9") {
		t.Fatalf("expected install confirmation, got %q", output)
	}
	if _, err := os.Stat(filepath.Join(installDir, "dockman")); err != nil {
		t.Fatalf("expected binary written: %v", err)
	}
}
