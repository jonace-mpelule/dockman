package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func ensureDocker() {
	cmd := exec.Command("docker", "--version")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Docker not installed or not in PATH")
		os.Exit(1)
	}
}

func runDocker(trail string, envPath string, inlineEnv map[string]string, autoPort bool, allowNoEnv bool, dryRun bool) error {
	if strings.TrimSpace(trail) == "" {
		return fmt.Errorf("no run trail provided")
	}

	envContent, path, err := loadRunEnv(envPath, inlineEnv, allowNoEnv)
	if err != nil {
		return err
	}

	trailArgs := splitArgs(trail)
	commands := []string{"run"}
	masked := map[int]bool{}

	// Prepend Port From Env if -p is not provided
	if autoPort && !hasPortFlag(trailArgs) {
		if _, ok := envContent["PORT"]; ok {
			commands = append(commands, "-p", fmt.Sprintf("%s:%s", envContent["PORT"], envContent["PORT"]))
		}
	}

	keys := make([]string, 0, len(envContent))
	for key := range envContent {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		commands = append(commands, "-e", fmt.Sprintf("%s=%s", key, envContent[key]))
		masked[len(commands)] = true
	}

	commands = append(commands, trailArgs...)

	fullCommand := append([]string{"docker"}, commands...)
	if dryRun {
		fmt.Printf("Dry run: %s\n", formatCommandMasked(fullCommand, masked))
		return nil
	}

	dockerCommand := exec.Command("docker", commands...)
	dockerCommand.Stdin = os.Stdin
	dockerCommand.Stdout = os.Stdout
	dockerCommand.Stderr = os.Stderr

	if path != "" {
		fmt.Printf("\n 🐳 Dockman Injected %v Variables From %v \n\n", len(envContent), path)
	}

	if err := dockerCommand.Run(); err != nil {
		return err
	}

	return nil
}

func loadRunEnv(envPath string, inlineEnv map[string]string, allowNoEnv bool) (map[string]string, string, error) {
	envContent := map[string]string{}
	path := ""
	if strings.TrimSpace(envPath) != "" {
		resolvedPath, err := resolveStringField("run.env_file", envPath)
		if err != nil {
			return nil, "", err
		}
		path = filepath.Join(resolvedPath)

		if _, err := os.ReadFile(path); err != nil {
			if allowNoEnv && os.IsNotExist(err) {
				path = ""
			} else {
				return nil, "", fmt.Errorf("error opening env file: %v", path)
			}
		} else {
			parsed, err := parseEnv(path)
			if err != nil {
				return nil, "", fmt.Errorf("error opening env file: %v", path)
			}
			envContent = parsed
		}
	}

	resolvedInlineEnv, err := resolveStringMap("run.env", inlineEnv, false)
	if err != nil {
		return nil, "", err
	}
	for _, assignment := range resolvedInlineEnv {
		envContent[assignment.Key] = assignment.Value
	}

	return envContent, path, nil
}

func buildDocker(cfg Config, overrides BuildOverrides, dryRun bool) error {
	plan, err := resolveBuildPlan(cfg, overrides)
	if err != nil {
		return err
	}

	fullCommand := append(plan.PreviewEnv, append([]string{"docker"}, plan.Args...)...)
	if dryRun {
		fmt.Printf("Dry run: %s\n", formatCommandMasked(fullCommand, plan.Masked))
		return nil
	}

	dockerCommand := exec.Command("docker", plan.Args...)
	dockerCommand.Stdin = os.Stdin
	dockerCommand.Stdout = os.Stdout
	dockerCommand.Stderr = os.Stderr
	dockerCommand.Env = append(os.Environ(), envAssignmentsToStrings(plan.Env)...)

	if err := dockerCommand.Run(); err != nil {
		return err
	}

	return nil
}

type buildPlan struct {
	Args       []string
	Env        []envAssignment
	PreviewEnv []string
	Masked     map[int]bool
}

func resolveBuildPlan(cfg Config, overrides BuildOverrides) (buildPlan, error) {
	tag := strings.TrimSpace(overrides.Tag)
	if tag == "" {
		tag = strings.TrimSpace(cfg.Build.Tag)
	}
	if tag == "" {
		tag = strings.TrimSpace(cfg.Image)
	}

	context := strings.TrimSpace(overrides.Context)
	if context == "" {
		context = strings.TrimSpace(cfg.Build.Context)
	}
	if context == "" {
		context = "."
	}

	dockerfile := strings.TrimSpace(overrides.File)
	if dockerfile == "" {
		dockerfile = strings.TrimSpace(cfg.Build.Dockerfile)
	}

	target := strings.TrimSpace(overrides.Target)
	if target == "" {
		target = strings.TrimSpace(cfg.Build.Target)
	}

	platform := strings.TrimSpace(overrides.Platform)
	if platform == "" {
		platform = strings.TrimSpace(cfg.Build.Platform)
	}

	extraArgs := strings.TrimSpace(overrides.Args)
	if extraArgs == "" {
		extraArgs = strings.TrimSpace(cfg.Build.ExtraArgs)
	}
	if extraArgs == "" {
		extraArgs = strings.TrimSpace(cfg.Build.LegacyArgs)
	}

	buildKit := true
	if cfg.Build.BuildKit != nil {
		buildKit = *cfg.Build.BuildKit
	}
	if overrides.BuildKit != nil {
		buildKit = *overrides.BuildKit
	}

	if !buildKit && (len(cfg.Build.Secrets) > 0 || len(cfg.Build.SSH) > 0) {
		return buildPlan{}, fmt.Errorf("buildkit must be enabled when using build.secrets or build.ssh")
	}

	resolvedContext, err := resolveStringField("build.context", context)
	if err != nil {
		return buildPlan{}, err
	}
	resolvedDockerfile, err := resolveStringField("build.dockerfile", dockerfile)
	if err != nil {
		return buildPlan{}, err
	}
	resolvedTag, err := resolveStringField("build.tag", tag)
	if err != nil {
		return buildPlan{}, err
	}
	resolvedTarget, err := resolveStringField("build.target", target)
	if err != nil {
		return buildPlan{}, err
	}
	resolvedPlatform, err := resolveStringField("build.platform", platform)
	if err != nil {
		return buildPlan{}, err
	}
	resolvedExtraArgs, err := resolveStringField("build.extra_args", extraArgs)
	if err != nil {
		return buildPlan{}, err
	}

	buildEnv, err := resolveStringMap("build.env", cfg.Build.Env, true)
	if err != nil {
		return buildPlan{}, err
	}

	env := make([]envAssignment, 0, len(buildEnv)+1)
	previewEnv := make([]string, 0, len(buildEnv)+1)
	masked := map[int]bool{}
	if buildKit {
		env = append(env, envAssignment{Key: "DOCKER_BUILDKIT", Value: "1"})
		previewEnv = append(previewEnv, "DOCKER_BUILDKIT=1")
	}
	for _, assignment := range buildEnv {
		env = append(env, assignment)
		previewEnv = append(previewEnv, fmt.Sprintf("%s=%s", assignment.Key, assignment.Value))
		masked[len(previewEnv)-1] = assignment.Sensitive
	}

	args := []string{"build"}
	commandOffset := len(previewEnv) + 1
	if resolvedTag != "" {
		args = append(args, "-t", resolvedTag)
	}
	if resolvedDockerfile != "" {
		args = append(args, "-f", resolvedDockerfile)
	}
	if resolvedTarget != "" {
		args = append(args, "--target", resolvedTarget)
	}
	if resolvedPlatform != "" {
		args = append(args, "--platform", resolvedPlatform)
	}

	noCache := cfg.Build.NoCache != nil && *cfg.Build.NoCache
	if overrides.NoCache != nil {
		noCache = *overrides.NoCache
	}
	if noCache {
		args = append(args, "--no-cache")
	}

	pull := cfg.Build.Pull != nil && *cfg.Build.Pull
	if overrides.Pull != nil {
		pull = *overrides.Pull
	}
	if pull {
		args = append(args, "--pull")
	}

	cacheFrom, err := resolveStringSlice("build.cache_from", cfg.Build.CacheFrom)
	if err != nil {
		return buildPlan{}, err
	}
	for _, value := range cacheFrom {
		args = append(args, "--cache-from", value)
	}

	cacheTo, err := resolveStringSlice("build.cache_to", cfg.Build.CacheTo)
	if err != nil {
		return buildPlan{}, err
	}
	for _, value := range cacheTo {
		args = append(args, "--cache-to", value)
	}

	buildArgKeys := make([]string, 0, len(cfg.Build.Args))
	for key := range cfg.Build.Args {
		buildArgKeys = append(buildArgKeys, key)
	}
	sort.Strings(buildArgKeys)
	for _, key := range buildArgKeys {
		value, err := resolveStringField(fmt.Sprintf("build.args.%s", key), cfg.Build.Args[key])
		if err != nil {
			return buildPlan{}, err
		}
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
		masked[len(args)-1+commandOffset] = true
	}

	secretKeys := make([]string, 0, len(cfg.Build.Secrets))
	for key := range cfg.Build.Secrets {
		secretKeys = append(secretKeys, key)
	}
	sort.Strings(secretKeys)
	for _, key := range secretKeys {
		secret := cfg.Build.Secrets[key]
		if strings.TrimSpace(secret.Env) == "" && strings.TrimSpace(secret.File) == "" {
			return buildPlan{}, fmt.Errorf("build.secrets.%s must set env or file", key)
		}
		if strings.TrimSpace(secret.Env) != "" && strings.TrimSpace(secret.File) != "" {
			return buildPlan{}, fmt.Errorf("build.secrets.%s cannot set both env and file", key)
		}

		if strings.TrimSpace(secret.Env) != "" {
			envName, err := resolveStringField(fmt.Sprintf("build.secrets.%s.env", key), secret.Env)
			if err != nil {
				return buildPlan{}, err
			}
			if _, ok := os.LookupEnv(envName); !ok {
				return buildPlan{}, fmt.Errorf("missing build secret env var %s for secret %s", envName, key)
			}
			args = append(args, "--secret", fmt.Sprintf("id=%s,env=%s", key, envName))
			continue
		}

		filePath, err := resolveStringField(fmt.Sprintf("build.secrets.%s.file", key), secret.File)
		if err != nil {
			return buildPlan{}, err
		}
		if _, err := os.Stat(filePath); err != nil {
			return buildPlan{}, fmt.Errorf("missing build secret file for secret %s: %s", key, filePath)
		}
		args = append(args, "--secret", fmt.Sprintf("id=%s,src=%s", key, filePath))
	}

	sshSpecs, err := resolveStringSlice("build.ssh", cfg.Build.SSH)
	if err != nil {
		return buildPlan{}, err
	}
	for _, ssh := range sshSpecs {
		args = append(args, "--ssh", ssh)
	}

	args = append(args, splitArgs(resolvedExtraArgs)...)
	args = append(args, resolvedContext)

	return buildPlan{
		Args:       args,
		Env:        env,
		PreviewEnv: previewEnv,
		Masked:     masked,
	}, nil
}
