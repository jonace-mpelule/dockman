package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

func main() {
	args := os.Args[1:]

	mode := "run"
	if len(args) > 0 {
		switch args[0] {
		case "init", "build", "run", "help", "version":
			mode = args[0]
			args = args[1:]
		}
	}

	options, args := extractGlobalOptions(args)
	if options.Help || mode == "help" {
		printHelp()
		return
	}
	if options.Version || mode == "version" {
		fmt.Printf("dockman %s\n", version)
		return
	}

	configPath := options.ConfigPath
	if configPath == "" {
		configPath = configPathForProfile(options.Profile)
	}

	switch mode {
	case "init":
		if err := writeDefaultConfig(configPath, options.Profile); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Created %s\n", configPath)
		return
	case "build":
		if !options.DryRun {
			ensureDocker()
		}

		cfg := Config{}
		if loadedCfg, err := loadConfig(configPath); err == nil {
			cfg = loadedCfg
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Fatal(err)
		}

		overrides := BuildOverrides{}
		for _, arg := range args {
			if after, ok := strings.CutPrefix(arg, "--tag="); ok {
				overrides.Tag = after
				continue
			}
			if after, ok := strings.CutPrefix(arg, "--context="); ok {
				overrides.Context = after
				continue
			}
			if after, ok := strings.CutPrefix(arg, "--file="); ok {
				overrides.File = after
				continue
			}
			if after, ok := strings.CutPrefix(arg, "--args="); ok {
				overrides.Args = after
				continue
			}
		}

		if err := buildDocker(cfg, overrides, options.DryRun); err != nil {
			log.Fatal(err)
		}
		return
	case "run":
		if !options.DryRun {
			ensureDocker()
		}

		cfg := Config{}
		cfgLoaded := false
		if loadedCfg, err := loadConfig(configPath); err == nil {
			cfg = loadedCfg
			cfgLoaded = true
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Fatal(err)
		}

		trail := ""
		envFile := ""
		passThrough := []string{}
		for _, arg := range args {
			if after, ok := strings.CutPrefix(arg, "--tc="); ok {
				trail = after
				continue
			}
			if after, ok := strings.CutPrefix(arg, "--env="); ok {
				envFile = after
				continue
			}
			if arg == "--no-port" {
				autoPort := false
				cfg.AutoPort = &autoPort
				continue
			}
			if after, ok := strings.CutPrefix(arg, "--auto-port="); ok {
				parsed, err := strconv.ParseBool(after)
				if err != nil {
					log.Fatalf("invalid --auto-port value: %s", after)
				}
				cfg.AutoPort = &parsed
				continue
			}
			passThrough = append(passThrough, arg)
		}

		if trail == "" {
			if len(passThrough) > 0 {
				trail = strings.Join(passThrough, " ")
			} else if cfgLoaded {
				trail = buildRunTrail(cfg)
			}
		}

		if trail == "" {
			log.Fatal("No trail provided")
		}

		if envFile == "" {
			if cfgLoaded && cfg.EnvFile != "" {
				envFile = cfg.EnvFile
			} else {
				if options.Profile != "" {
					envFile = fmt.Sprintf(".env.%s", options.Profile)
				} else {
					envFile = defaultEnvFile
				}
			}
		}

		autoPort := true
		if cfg.AutoPort != nil {
			autoPort = *cfg.AutoPort
		}

		if err := runDocker(trail, envFile, autoPort, options.DryRun); err != nil {
			log.Fatal(err)
		}
		return
	default:
		log.Fatal("Unknown command")
	}
}
