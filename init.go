package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var errInitCancelled = errors.New("init cancelled")

var terminalChecker = func(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

var interactiveInitRunner = runInteractiveInit

func shouldUseInteractiveInit(options GlobalOptions) bool {
	if options.InitPlain || options.InitYes {
		return false
	}
	return terminalChecker(os.Stdin) && terminalChecker(os.Stdout)
}

func runInit(path string, profile string, options GlobalOptions) error {
	if shouldUseInteractiveInit(options) {
		cfg, overwrite, err := interactiveInitRunner(path, profile)
		if err != nil {
			return err
		}
		if !overwrite {
			exists, err := configExists(path)
			if err != nil {
				return err
			}
			if exists {
				return errInitCancelled
			}
		}
		return writeConfig(path, cfg, overwrite)
	}

	return writeConfig(path, defaultConfig(profile), false)
}

func runInteractiveInit(path string, profile string) (Config, bool, error) {
	cfg := defaultConfig(profile)
	exists, err := configExists(path)
	if err != nil {
		return Config{}, false, err
	}

	image := cfg.Image
	envFile := cfg.Run.EnvFile
	runArgs := cfg.Run.Args
	name := cfg.Run.Name
	zeroDowntime := cfg.Run.ZeroDowntime != nil && *cfg.Run.ZeroDowntime
	readiness := cfg.Run.Readiness
	context := cfg.Build.Context
	dockerfile := cfg.Build.Dockerfile
	tag := cfg.Build.Tag
	buildKit := cfg.Build.BuildKit != nil && *cfg.Build.BuildKit

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	subtleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	totalSteps := 11

	form := huh.NewForm(
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 1, totalSteps, "Image name", "Create a Docker workflow config with guided defaults."),
			huh.NewInput().Title("Image name").Description("Image to run by default.").Value(&image),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 2, totalSteps, "Env file", ""),
			huh.NewInput().Title("Env file").Description("Runtime env file path.").Value(&envFile),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 3, totalSteps, "Run args", ""),
			huh.NewInput().Title("Run args").Description("Arguments placed before the image, for example --rm.").Value(&runArgs),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 4, totalSteps, "Managed app name", ""),
			huh.NewInput().Title("Managed app name").Description("Container/app name used by start, restart, and upgrade.").Value(&name),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 5, totalSteps, "Zero-downtime restarts", ""),
			huh.NewConfirm().Title("Enable zero-downtime restarts?").Value(&zeroDowntime),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 6, totalSteps, "Readiness mode", ""),
			huh.NewSelect[string]().
				Title("Readiness mode").
				Description("Current supported mode for zero-downtime checks.").
				Options(
					huh.NewOption("healthcheck", "healthcheck"),
				).
				Value(&readiness),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 7, totalSteps, "Build context", ""),
			huh.NewInput().Title("Build context").Description("Directory passed to docker build.").Value(&context),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 8, totalSteps, "Dockerfile", ""),
			huh.NewInput().Title("Dockerfile").Description("Dockerfile path for builds.").Value(&dockerfile),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 9, totalSteps, "Build tag", ""),
			huh.NewInput().Title("Build tag").Description("Tag used by dockman build.").Value(&tag),
		),
		huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 10, totalSteps, "BuildKit", ""),
			huh.NewConfirm().Title("Enable BuildKit?").Value(&buildKit),
		),
	)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return Config{}, false, errInitCancelled
		}
		return Config{}, false, err
	}

	cfg.Image = strings.TrimSpace(image)
	cfg.Run.EnvFile = strings.TrimSpace(envFile)
	cfg.Run.Args = strings.TrimSpace(runArgs)
	cfg.Run.Name = strings.TrimSpace(name)
	cfg.Run.ZeroDowntime = boolPtr(zeroDowntime)
	cfg.Run.Readiness = strings.TrimSpace(readiness)
	cfg.Build.Context = strings.TrimSpace(context)
	cfg.Build.Dockerfile = strings.TrimSpace(dockerfile)
	cfg.Build.Tag = strings.TrimSpace(tag)
	cfg.Build.BuildKit = boolPtr(buildKit)

	summary := formatInitSummary(path, cfg, exists)
	confirm := true
	overwrite := false

	var reviewGroup *huh.Group
	if exists {
		reviewGroup = huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 11, totalSteps, "Review", summary),
			huh.NewConfirm().
				Title("Overwrite the existing config?").
				Description("Choose no to cancel without changing anything.").
				Affirmative("Overwrite").
				Negative("Cancel").
				Value(&overwrite),
		)
	} else {
		reviewGroup = huh.NewGroup(
			initBrandNote(headerStyle, subtleStyle, path, profile, 11, totalSteps, "Review", summary),
			huh.NewConfirm().
				Title("Write this config?").
				Affirmative("Write").
				Negative("Cancel").
				Value(&confirm),
		)
	}

	reviewForm := huh.NewForm(reviewGroup)
	if err := reviewForm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return Config{}, false, errInitCancelled
		}
		return Config{}, false, err
	}

	if exists && !overwrite {
		return Config{}, false, errInitCancelled
	}
	if !exists && !confirm {
		return Config{}, false, errInitCancelled
	}

	return cfg, overwrite, nil
}

func initBrandNote(headerStyle lipgloss.Style, subtleStyle lipgloss.Style, path string, profile string, step int, totalSteps int, title string, body string) *huh.Note {
	lines := []string{
		headerStyle.Render("Dockman Init"),
		subtleStyle.Render(fmt.Sprintf("Step %d/%d", step, totalSteps)),
		subtleStyle.Render(fmt.Sprintf("Target: %s", path)),
		subtleStyle.Render(fmt.Sprintf("Profile: %s", profileLabel(profile))),
	}
	if strings.TrimSpace(title) != "" {
		lines = append(lines, "", title)
	}
	if strings.TrimSpace(body) != "" {
		lines = append(lines, body)
	}
	return huh.NewNote().Title("").Description(strings.Join(lines, "\n"))
}

func formatInitSummary(path string, cfg Config, exists bool) string {
	lines := []string{
		fmt.Sprintf("Path: %s", path),
		fmt.Sprintf("Image: %s", cfg.Image),
		fmt.Sprintf("Env file: %s", cfg.Run.EnvFile),
		fmt.Sprintf("Run args: %s", cfg.Run.Args),
		fmt.Sprintf("Managed name: %s", cfg.Run.Name),
		fmt.Sprintf("Zero downtime: %t", cfg.Run.ZeroDowntime != nil && *cfg.Run.ZeroDowntime),
		fmt.Sprintf("Readiness: %s", cfg.Run.Readiness),
		fmt.Sprintf("Build context: %s", cfg.Build.Context),
		fmt.Sprintf("Dockerfile: %s", cfg.Build.Dockerfile),
		fmt.Sprintf("Build tag: %s", cfg.Build.Tag),
		fmt.Sprintf("BuildKit: %t", cfg.Build.BuildKit != nil && *cfg.Build.BuildKit),
	}
	if exists {
		lines = append(lines, "An existing config will be replaced only if you confirm overwrite.")
	}
	return strings.Join(lines, "\n")
}

func profileLabel(profile string) string {
	if strings.TrimSpace(profile) == "" {
		return "default"
	}
	return profile
}

func printInitSuccess(path string, profile string) {
	const orange = "\033[38;5;214m"
	const reset = "\033[0m"

	fmt.Printf(orange+`  ____             __
 / __ \____  _____/ /_____ ___  ____ _____
/ / / / __ \/ ___/ //_/ _ `+"`"+`__ \/ __ `+"`"+`/ __ \
/ /_/ / /_/ / /__/ ,< /  __/ / / /_/ / / / /
\____/\____/\___/_/|_|\___/_/ /_/\__,_/_/ /_/`+reset+`

Dockman init complete
Config    %s
Profile   %s

Next:
  - edit %s if you want to change defaults
  - run dockman doctor to validate the config
  - run dockman to use the config-driven flow
`, path, profileLabel(profile), path)
}
