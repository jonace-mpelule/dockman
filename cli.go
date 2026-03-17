package main

import (
	"fmt"
	"strconv"
	"strings"
)

var version = "dev"

type GlobalOptions struct {
	ConfigPath string
	Profile    string
	DryRun     bool
	Help       bool
	Update     bool
	Version    bool
	AllowNoEnv bool
}

func splitArgs(input string) []string {
	if strings.TrimSpace(input) == "" {
		return []string{}
	}
	return strings.Fields(input)
}

func hasPortFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-p" || arg == "--publish" {
			return true
		}
		if strings.HasPrefix(arg, "-p") || strings.HasPrefix(arg, "--publish=") {
			return true
		}
	}
	return false
}

func shellEscape(arg string) string {
	if arg == "" {
		return "\"\""
	}
	if strings.ContainsAny(arg, " \t\n\"'\\$") {
		return strconv.Quote(arg)
	}
	return arg
}

func formatCommand(args []string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, shellEscape(arg))
	}
	return strings.Join(escaped, " ")
}

func extractGlobalOptions(args []string) (GlobalOptions, []string) {
	options := GlobalOptions{}
	filtered := make([]string, 0, len(args))

	for _, arg := range args {
		if after, ok := strings.CutPrefix(arg, "--config="); ok {
			options.ConfigPath = after
			continue
		}
		if after, ok := strings.CutPrefix(arg, "--profile="); ok {
			options.Profile = after
			continue
		}
		if arg == "--dry-run" {
			options.DryRun = true
			continue
		}
		if arg == "--help" || arg == "-h" {
			options.Help = true
			continue
		}
		if arg == "--update" {
			options.Update = true
			continue
		}
		if arg == "--version" {
			options.Version = true
			continue
		}
		if arg == "--allow-no-env" {
			options.AllowNoEnv = true
			continue
		}
		filtered = append(filtered, arg)
	}

	return options, filtered
}

func printHelp() {
	fmt.Print(`Dockman - Docker run/build helper with env injection

Usage:
  dockman [run] [--tc="..."] [--env="..."] [--no-port] [--allow-no-env] [--dry-run] [--profile=NAME] [--config=PATH]
  dockman start [--allow-no-env] [--dry-run] [--profile=NAME] [--config=PATH]
  dockman stop [--dry-run] [--profile=NAME] [--config=PATH]
  dockman restart [--allow-no-env] [--dry-run] [--profile=NAME] [--config=PATH]
  dockman upgrade [--image="..."] [--allow-no-env] [--dry-run] [--profile=NAME] [--config=PATH]
  dockman build [--tag="..."] [--context="..."] [--file="..."] [--target="..."] [--platform="..."] [--args="..."] [--no-cache] [--pull] [--buildkit=true|false] [--dry-run] [--profile=NAME] [--config=PATH]
  dockman doctor [--dry-run] [--profile=NAME] [--config=PATH]
  dockman init [--profile=NAME] [--config=PATH]
  dockman --help
  dockman --update
  dockman --version

Notes:
  - If no args are provided, Dockman uses dockman.json (or dockman.<profile>.json).
  - Use --profile to switch config and default env file (.env.<profile>).
  - Use --allow-no-env to continue when the resolved env file does not exist.
  - Use --no-port to disable automatic PORT mapping.
  - Use --dry-run to print the docker command without executing.
  - Use --update to reinstall the latest Dockman release from GitHub.
  - start/stop/restart/upgrade require run.name in config for managed lifecycle mode.
  - BuildKit is enabled by default for dockman build.
  - doctor validates and rewrites older config files to the current schema.
`)
}
