package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func ensureDocker() {
	cmd := exec.Command("docker", "--version")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Docker not installed or not in PATH")
		os.Exit(1)
	}
}

func runDocker(trail string, envPath string, autoPort bool, dryRun bool) error {
	if strings.TrimSpace(trail) == "" {
		return fmt.Errorf("no run trail provided")
	}

	if envPath == "" {
		envPath = defaultEnvFile
	}

	path := filepath.Join(envPath)

	_, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error opening env file: %v", path)
	}

	envContent, err := parseEnv(path)
	if err != nil {
		return fmt.Errorf("error opening env file: %v", path)
	}

	envArgs := []string{}
	for k, v := range envContent {
		envArgs = append(envArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	trailArgs := splitArgs(trail)

	// Prepend Port From Env if -p is not provided
	if autoPort && !hasPortFlag(trailArgs) {
		if _, ok := envContent["PORT"]; ok {
			envArgs = append([]string{
				"-p", fmt.Sprintf("%s:%s", envContent["PORT"], envContent["PORT"]),
			}, envArgs...)
		}
	}

	commands := []string{"run"}
	commands = append(commands, envArgs...)
	commands = append(commands, trailArgs...)

	fullCommand := append([]string{"docker"}, commands...)
	if dryRun {
		fmt.Printf("Dry run: %s\n", formatCommand(fullCommand))
		return nil
	}

	dockerCommand := exec.Command("docker", commands...)
	dockerCommand.Stdin = os.Stdin
	dockerCommand.Stdout = os.Stdout
	dockerCommand.Stderr = os.Stderr

	fmt.Printf("\n 🐳 Dockman Injected %v Variables From %v \n\n", len(envContent), path)

	if err := dockerCommand.Run(); err != nil {
		return err
	}

	return nil
}

func buildDocker(cfg Config, overrides BuildOverrides, dryRun bool) error {
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

	extraArgs := strings.TrimSpace(overrides.Args)
	if extraArgs == "" {
		extraArgs = strings.TrimSpace(cfg.Build.Args)
	}

	args := []string{"build"}
	if tag != "" {
		args = append(args, "-t", tag)
	}
	if dockerfile != "" {
		args = append(args, "-f", dockerfile)
	}
	args = append(args, splitArgs(extraArgs)...)
	args = append(args, context)

	fullCommand := append([]string{"docker"}, args...)
	if dryRun {
		fmt.Printf("Dry run: %s\n", formatCommand(fullCommand))
		return nil
	}

	dockerCommand := exec.Command("docker", args...)
	dockerCommand.Stdin = os.Stdin
	dockerCommand.Stdout = os.Stdout
	dockerCommand.Stderr = os.Stderr

	if err := dockerCommand.Run(); err != nil {
		return err
	}

	return nil
}
