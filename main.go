package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func parseEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	env := make(map[string]string)

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip Empty Lines
		if line == " " || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)

		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		env[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return env, nil

}

func main() {
	// Check if Docker exists
	cmd := exec.Command("docker", "--version")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Docker not installed or not in PATH")
		os.Exit(1)
	}

	args := os.Args[1:]

	if len(args) == 0 {
		log.Fatal("No arguments provided")
		return
	}

	result := make(map[string]string)

	wanted := map[string]string{
		"--tc=":  "trail",
		"--env=": "envFile",
	}

	for _, arg := range args {
		for prefix, key := range wanted {
			if after, ok := strings.CutPrefix(arg, prefix); ok {
				result[key] = after
			}
		}
	}

	if len(result["trail"]) == 0 {
		fmt.Println("No trail provided")
		os.Exit(1)
	}

	_envPath := result["envFile"]
	if _envPath == "" {
		_envPath = ".env"
	}

	path := filepath.Join(_envPath)

	_, err := os.ReadFile(path)

	if err != nil {
		fmt.Printf("Error Opening Env File: %v \n", path)
		os.Exit(1)
	}

	envContent, err := parseEnv(path)

	if err != nil {
		fmt.Printf("Error Opening Env File: %v \n", path)
		os.Exit(1)
	}

	envArgs := []string{}

	for k, v := range envContent {
		envArgs = append(envArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	commands := []string{
		"run",
	}

	trail := strings.SplitN(result["trail"], " ", 200)

	// Prepend Port From Env if -p is not provided
	if !strings.Contains(result["trail"], "-p") {
		if _, ok := envContent["PORT"]; ok {
			envArgs = append([]string{
				"-p", fmt.Sprintf("%s:%s", envContent["PORT"], envContent["PORT"]),
			}, envArgs...)
		}

	}

	commands = append(commands, envArgs...)
	commands = append(commands, trail...)

	dockerCommand := exec.Command("docker", commands...)

	dockerCommand.Stdin = os.Stdin
	dockerCommand.Stdout = os.Stdout
	dockerCommand.Stderr = os.Stderr

	fmt.Printf("\n 🐳 Dockman Injected %v Variables From %v \n\n", len(envContent), path)

	if err := dockerCommand.Run(); err != nil {
		panic(err)
	}

	dockerCommand.Wait()

}
