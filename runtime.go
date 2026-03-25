package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	dockmanManagedLabel = "dockman.managed=true"
	dockmanAppLabelKey  = "dockman.app"
	dockmanRoleLabelKey = "dockman.role"
	dockmanRoleApp      = "app"
	dockmanRoleProxy    = "proxy"
	proxyImage          = "nginx:alpine"
	healthWaitTimeout   = 120 * time.Second
)

type managedRuntimeOptions struct {
	DryRun     bool
	AllowNoEnv bool
	Image      string
}

type managedRuntime struct {
	Name            string
	LegacyName      string
	Image           string
	PreImageArgs    []string
	PostImageArgs   []string
	Env             map[string]string
	ResolvedEnvPath string
	AutoPort        bool
	ZeroDowntime    bool
	Readiness       string
	PublishSpec     string
	ListenPort      string
	HostPort        string
	NetworkName     string
	ProxyName       string
	ConfigDir       string
	ProxyConfigPath string
}

func runManagedLifecycle(action string, cfg Config, opts managedRuntimeOptions) error {
	rt, err := resolveManagedRuntime(cfg, opts)
	if err != nil {
		return err
	}

	if opts.DryRun {
		return printManagedDryRun(action, rt, opts)
	}

	switch action {
	case "start":
		return executeManagedStart(rt)
	case "stop":
		return executeManagedStop(rt)
	case "restart":
		return executeManagedRestart(rt, false)
	case "upgrade":
		return executeManagedRestart(rt, true)
	default:
		return fmt.Errorf("unknown lifecycle action: %s", action)
	}
}

func resolveManagedRuntime(cfg Config, opts managedRuntimeOptions) (managedRuntime, error) {
	name := strings.TrimSpace(cfg.Run.Name)
	if name == "" {
		return managedRuntime{}, fmt.Errorf("run.name is required for managed lifecycle commands")
	}

	image := strings.TrimSpace(cfg.Image)
	if strings.TrimSpace(opts.Image) != "" {
		image = strings.TrimSpace(opts.Image)
	}
	resolvedImage, err := resolveStringField("image", image)
	if err != nil {
		return managedRuntime{}, err
	}
	if resolvedImage == "" {
		return managedRuntime{}, fmt.Errorf("image is required for managed lifecycle commands")
	}

	envPath := runEnvFile(cfg)
	if envPath == "" {
		envPath = defaultEnvFile
	}
	envContent, resolvedEnvPath, err := loadRunEnv(envPath, cfg.Run.Env, opts.AllowNoEnv)
	if err != nil {
		return managedRuntime{}, err
	}

	preArgs, postArgs, err := resolveManagedRunArgs(cfg, resolvedImage)
	if err != nil {
		return managedRuntime{}, err
	}

	autoPort := true
	if runAutoPort(cfg) != nil {
		autoPort = *runAutoPort(cfg)
	}
	zeroDowntime := cfg.Run.ZeroDowntime != nil && *cfg.Run.ZeroDowntime
	readiness := strings.TrimSpace(cfg.Run.Readiness)
	if readiness == "" {
		readiness = "healthcheck"
	}

	sanitizedArgs, legacyName, err := sanitizeManagedArgs(preArgs, zeroDowntime)
	if err != nil {
		return managedRuntime{}, err
	}

	publishSpec := ""
	listenPort := ""
	hostPort := ""
	if zeroDowntime {
		var found bool
		sanitizedArgs, publishSpec, listenPort, found, err = extractPublishSpec(sanitizedArgs)
		if err != nil {
			return managedRuntime{}, err
		}
		if !found && autoPort {
			if port := strings.TrimSpace(envContent["PORT"]); port != "" {
				publishSpec = fmt.Sprintf("%s:%s", port, port)
				hostPort = port
				listenPort = port
			}
		}
		if listenPort == "" {
			return managedRuntime{}, fmt.Errorf("zero-downtime mode requires either PORT in runtime env or an explicit TCP publish flag")
		}
		if hostPort == "" && publishSpec != "" {
			hostPort, err = hostPortFromPublishSpec(publishSpec)
			if err != nil {
				return managedRuntime{}, err
			}
		}
	} else {
		publishSpec, hostPort, _, _, err = inspectPublishSpec(sanitizedArgs)
		if err != nil {
			return managedRuntime{}, err
		}
		if publishSpec == "" && autoPort {
			if port := strings.TrimSpace(envContent["PORT"]); port != "" {
				publishSpec = fmt.Sprintf("%s:%s", port, port)
				hostPort = port
			}
		}
	}

	configDir := filepath.Join(os.TempDir(), "dockman", sanitizeResourceName(name))

	return managedRuntime{
		Name:            name,
		LegacyName:      legacyName,
		Image:           resolvedImage,
		PreImageArgs:    sanitizedArgs,
		PostImageArgs:   postArgs,
		Env:             envContent,
		ResolvedEnvPath: resolvedEnvPath,
		AutoPort:        autoPort,
		ZeroDowntime:    zeroDowntime,
		Readiness:       readiness,
		PublishSpec:     publishSpec,
		ListenPort:      listenPort,
		HostPort:        hostPort,
		NetworkName:     fmt.Sprintf("%s-net", sanitizeResourceName(name)),
		ProxyName:       fmt.Sprintf("%s-proxy", sanitizeResourceName(name)),
		ConfigDir:       configDir,
		ProxyConfigPath: filepath.Join(configDir, "nginx.conf"),
	}, nil
}

func resolveManagedRunArgs(cfg Config, image string) ([]string, []string, error) {
	rawArgs, err := resolveStringField("run.args", runArgs(cfg))
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(rawArgs) == "" {
		return nil, nil, nil
	}

	if strings.Contains(rawArgs, "{image}") {
		withImage := strings.ReplaceAll(rawArgs, "{image}", image)
		args := splitArgs(withImage)
		for i, arg := range args {
			if arg == image {
				return args[:i], args[i+1:], nil
			}
		}
		return nil, nil, fmt.Errorf("run.args placeholder must resolve to a distinct image token")
	}

	return splitArgs(rawArgs), nil, nil
}

func sanitizeManagedArgs(args []string, zeroDowntime bool) ([]string, string, error) {
	out := make([]string, 0, len(args))
	legacyName := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--rm" || arg == "-d" || arg == "--detach":
			continue
		case arg == "--name":
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("--name requires a value")
			}
			legacyName = args[i+1]
			i++
			continue
		case strings.HasPrefix(arg, "--name="):
			legacyName = strings.TrimPrefix(arg, "--name=")
			continue
		case zeroDowntime && (arg == "--network" || arg == "--net"):
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("%s requires a value", arg)
			}
			i++
			continue
		case zeroDowntime && (strings.HasPrefix(arg, "--network=") || strings.HasPrefix(arg, "--net=")):
			continue
		default:
			out = append(out, arg)
		}
	}
	return out, strings.TrimSpace(legacyName), nil
}

func extractPublishSpec(args []string) ([]string, string, string, bool, error) {
	out := make([]string, 0, len(args))
	publishSpec := ""
	listenPort := ""
	found := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		spec := ""
		switch {
		case arg == "-p" || arg == "--publish":
			if i+1 >= len(args) {
				return nil, "", "", false, fmt.Errorf("%s requires a value", arg)
			}
			spec = args[i+1]
			i++
		case strings.HasPrefix(arg, "--publish="):
			spec = strings.TrimPrefix(arg, "--publish=")
		case strings.HasPrefix(arg, "-p") && len(arg) > 2:
			spec = strings.TrimPrefix(arg, "-p")
		default:
			out = append(out, arg)
			continue
		}

		if found {
			return nil, "", "", false, fmt.Errorf("zero-downtime mode supports exactly one published TCP port")
		}

		_, port, err := publishPortsFromSpec(spec)
		if err != nil {
			return nil, "", "", false, err
		}

		found = true
		publishSpec = spec
		listenPort = port
	}

	return out, publishSpec, listenPort, found, nil
}

func inspectPublishSpec(args []string) (string, string, string, bool, error) {
	found := false
	spec := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-p" || arg == "--publish":
			if i+1 >= len(args) {
				return "", "", "", false, fmt.Errorf("%s requires a value", arg)
			}
			spec = args[i+1]
			found = true
			i++
		case strings.HasPrefix(arg, "--publish="):
			spec = strings.TrimPrefix(arg, "--publish=")
			found = true
		case strings.HasPrefix(arg, "-p") && len(arg) > 2:
			spec = strings.TrimPrefix(arg, "-p")
			found = true
		}
		if found {
			break
		}
	}
	if !found {
		return "", "", "", false, nil
	}
	hostPort, containerPort, err := publishPortsFromSpec(spec)
	if err != nil {
		return "", "", "", false, err
	}
	return spec, hostPort, containerPort, true, nil
}

func publishPortsFromSpec(spec string) (string, string, error) {
	protocol := ""
	base := spec
	if before, after, ok := strings.Cut(spec, "/"); ok {
		base = before
		protocol = after
	}
	if protocol != "" && protocol != "tcp" {
		return "", "", fmt.Errorf("zero-downtime mode only supports TCP publish flags")
	}

	parts := strings.Split(base, ":")
	containerPort := strings.TrimSpace(parts[len(parts)-1])
	if containerPort == "" {
		return "", "", fmt.Errorf("invalid publish spec: %s", spec)
	}
	hostPort := containerPort
	if len(parts) >= 2 {
		hostPort = strings.TrimSpace(parts[len(parts)-2])
	}
	if hostPort == "" {
		return "", "", fmt.Errorf("invalid publish spec: %s", spec)
	}
	return hostPort, containerPort, nil
}

func hostPortFromPublishSpec(spec string) (string, error) {
	hostPort, _, err := publishPortsFromSpec(spec)
	if err != nil {
		return "", err
	}
	return hostPort, nil
}

func managedRunArgs(rt managedRuntime, containerName string, useProxy bool) []string {
	args := []string{"run", "-d", "--name", containerName}
	args = append(args, managedLabels(rt.Name, dockmanRoleApp)...)
	if useProxy {
		args = append(args, "--network", rt.NetworkName)
	}

	if !useProxy && rt.AutoPort && !hasPortFlag(rt.PreImageArgs) {
		if port := strings.TrimSpace(rt.Env["PORT"]); port != "" {
			args = append(args, "-p", fmt.Sprintf("%s:%s", port, port))
		}
	}

	envKeys := make([]string, 0, len(rt.Env))
	for key := range rt.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, rt.Env[key]))
	}

	args = append(args, rt.PreImageArgs...)
	args = append(args, rt.Image)
	args = append(args, rt.PostImageArgs...)
	return args
}

func managedLabels(app string, role string) []string {
	return []string{
		"--label", dockmanManagedLabel,
		"--label", fmt.Sprintf("%s=%s", dockmanAppLabelKey, app),
		"--label", fmt.Sprintf("%s=%s", dockmanRoleLabelKey, role),
	}
}

func executeManagedStart(rt managedRuntime) error {
	if rt.ZeroDowntime {
		return executeManagedReplacement(rt, false)
	}

	if current, _ := currentManagedContainer(rt.Name, dockmanRoleApp); current != "" {
		return fmt.Errorf("managed app %s is already running", rt.Name)
	}

	if err := cleanupAdoptableContainers(rt, false); err != nil {
		return err
	}
	if err := ensureNoPortConflict(rt, false); err != nil {
		return err
	}
	return runDockerCommand(managedRunArgs(rt, rt.Name, false)...)
}

func executeManagedStop(rt managedRuntime) error {
	if err := stopAndRemoveManagedContainers(rt.Name, dockmanRoleApp); err != nil {
		return err
	}
	if err := stopAndRemoveManagedContainers(rt.Name, dockmanRoleProxy); err != nil {
		return err
	}
	return nil
}

func executeManagedRestart(rt managedRuntime, pull bool) error {
	if rt.ZeroDowntime {
		return executeManagedReplacement(rt, pull)
	}

	if pull {
		if err := runDockerCommand("pull", rt.Image); err != nil {
			return err
		}
	}
	if err := cleanupAdoptableContainers(rt, false); err != nil {
		return err
	}
	if err := ensureNoPortConflict(rt, false); err != nil {
		return err
	}
	return runDockerCommand(managedRunArgs(rt, rt.Name, false)...)
}

func executeManagedReplacement(rt managedRuntime, pull bool) error {
	if pull {
		if err := runDockerCommand("pull", rt.Image); err != nil {
			return err
		}
	}
	if rt.Readiness != "healthcheck" {
		return fmt.Errorf("unsupported run.readiness %q", rt.Readiness)
	}
	if err := ensureHealthcheck(rt.Image); err != nil {
		return err
	}
	if err := ensureDockerNetwork(rt.NetworkName); err != nil {
		return err
	}
	if err := cleanupAdoptableContainers(rt, true); err != nil {
		return err
	}

	newRevision := fmt.Sprintf("%s-%d", sanitizeResourceName(rt.Name), time.Now().Unix())
	currentApp, _ := currentManagedContainer(rt.Name, dockmanRoleApp)

	if err := runDockerCommand(managedRunArgs(rt, newRevision, true)...); err != nil {
		return err
	}
	if err := waitForHealthy(newRevision, healthWaitTimeout); err != nil {
		_ = runDockerCommand("rm", "-f", newRevision)
		return err
	}
	if err := writeProxyConfig(rt.ProxyConfigPath, rt.ListenPort, newRevision); err != nil {
		_ = runDockerCommand("rm", "-f", newRevision)
		return err
	}

	proxyState, err := containerStateByName(rt.ProxyName)
	if err != nil {
		_ = runDockerCommand("rm", "-f", newRevision)
		return err
	}
	if proxyState == "exited" {
		if err := runDockerCommand("rm", "-f", rt.ProxyName); err != nil {
			_ = runDockerCommand("rm", "-f", newRevision)
			return err
		}
		proxyState = ""
	}
	if proxyState == "" {
		if err := ensureNoPortConflict(rt, true); err != nil {
			_ = runDockerCommand("rm", "-f", newRevision)
			return err
		}
		if err := startProxy(rt); err != nil {
			_ = runDockerCommand("rm", "-f", newRevision)
			return err
		}
	} else {
		if err := runDockerCommand("exec", rt.ProxyName, "nginx", "-s", "reload"); err != nil {
			_ = runDockerCommand("rm", "-f", newRevision)
			return err
		}
	}

	if currentApp != "" && currentApp != newRevision {
		if err := runDockerCommand("rm", "-f", currentApp); err != nil {
			return err
		}
	}
	return nil
}

func startProxy(rt managedRuntime) error {
	if err := os.MkdirAll(rt.ConfigDir, 0o755); err != nil {
		return err
	}
	args := []string{
		"run", "-d",
		"--name", rt.ProxyName,
	}
	args = append(args, managedLabels(rt.Name, dockmanRoleProxy)...)
	args = append(args,
		"--network", rt.NetworkName,
		"-p", rt.PublishSpec,
		"-v", fmt.Sprintf("%s:/etc/nginx/nginx.conf:ro", rt.ProxyConfigPath),
		proxyImage,
	)
	return runDockerCommand(args...)
}

func ensureHealthcheck(image string) error {
	out, err := runDockerOutput("image", "inspect", "--format", "{{if .Config.Healthcheck}}present{{else}}missing{{end}}", image)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "present" {
		return fmt.Errorf("zero-downtime mode requires a Docker HEALTHCHECK on image %s", image)
	}
	return nil
}

func ensureDockerNetwork(name string) error {
	if _, err := runDockerOutput("network", "inspect", name); err == nil {
		return nil
	}
	return runDockerCommand("network", "create", name)
}

func currentManagedContainer(app string, role string) (string, error) {
	out, err := runDockerOutput(
		"ps", "--filter", fmt.Sprintf("label=%s", dockmanManagedLabel),
		"--filter", fmt.Sprintf("label=%s=%s", dockmanAppLabelKey, app),
		"--filter", fmt.Sprintf("label=%s=%s", dockmanRoleLabelKey, role),
		"--format", "{{.Names}}",
	)
	if err != nil {
		return "", nil
	}
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) == 0 {
		return "", nil
	}
	return lines[0], nil
}

func stopAndRemoveManagedContainers(app string, role string) error {
	out, err := runDockerOutput(
		"ps", "-aq",
		"--filter", fmt.Sprintf("label=%s", dockmanManagedLabel),
		"--filter", fmt.Sprintf("label=%s=%s", dockmanAppLabelKey, app),
		"--filter", fmt.Sprintf("label=%s=%s", dockmanRoleLabelKey, role),
	)
	if err != nil {
		return nil
	}
	ids := strings.Fields(strings.TrimSpace(out))
	for _, id := range ids {
		if err := runDockerCommand("rm", "-f", id); err != nil {
			return err
		}
	}
	return nil
}

func removeManagedContainerByName(name string) error {
	exists, err := containerExistsByName(name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return runDockerCommand("rm", "-f", name)
}

func containerExistsByName(name string) (bool, error) {
	out, err := runDockerOutput("ps", "-aq", "--filter", fmt.Sprintf("name=^%s$", name))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func containerStateByName(name string) (string, error) {
	out, err := runDockerOutput("ps", "-a", "--filter", fmt.Sprintf("name=^%s$", name), "--format", "{{.State}}")
	if err != nil {
		return "", err
	}
	lines := strings.Fields(strings.TrimSpace(out))
	if len(lines) == 0 {
		return "", nil
	}
	return lines[0], nil
}

func cleanupAdoptableContainers(rt managedRuntime, zeroDowntime bool) error {
	if err := stopAndRemoveManagedContainers(rt.Name, dockmanRoleApp); err != nil {
		return err
	}
	if !zeroDowntime {
		if err := stopAndRemoveManagedContainers(rt.Name, dockmanRoleProxy); err != nil {
			return err
		}
	}
	for _, name := range adoptableNames(rt) {
		if err := removeManagedContainerByName(name); err != nil {
			return err
		}
	}
	return nil
}

func adoptableNames(rt managedRuntime) []string {
	names := []string{strings.TrimSpace(rt.Name)}
	if legacy := strings.TrimSpace(rt.LegacyName); legacy != "" && legacy != rt.Name {
		names = append(names, legacy)
	}
	return names
}

func ensureNoPortConflict(rt managedRuntime, zeroDowntime bool) error {
	hostPort := strings.TrimSpace(rt.HostPort)
	if hostPort == "" {
		return nil
	}

	occupiedName, err := conflictingPublishedContainer(hostPort)
	if err != nil {
		return err
	}
	if occupiedName == "" {
		return nil
	}

	allowed := map[string]bool{}
	for _, name := range adoptableNames(rt) {
		allowed[name] = true
	}
	if zeroDowntime {
		allowed[rt.ProxyName] = true
	}
	if allowed[occupiedName] {
		return nil
	}

	return fmt.Errorf("port %s is already allocated by container %s; stop/remove that container or change the published port for %s", hostPort, occupiedName, rt.Name)
}

func conflictingPublishedContainer(hostPort string) (string, error) {
	out, err := runDockerOutput("ps", "--filter", fmt.Sprintf("publish=%s", hostPort), "--format", "{{.Names}}")
	if err != nil {
		return "", nil
	}
	names := strings.Fields(strings.TrimSpace(out))
	if len(names) == 0 {
		return "", nil
	}
	return names[0], nil
}

func waitForHealthy(container string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := runDockerOutput("inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}", container)
		if err != nil {
			return err
		}
		switch strings.TrimSpace(out) {
		case "healthy":
			return nil
		case "unhealthy", "none":
			return fmt.Errorf("replacement container %s did not become healthy", container)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timed out waiting for replacement container %s to become healthy", container)
}

func writeProxyConfig(path string, port string, upstream string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	config := fmt.Sprintf(`worker_processes auto;
events {}
stream {
    upstream dockman_upstream {
        server %s:%s;
    }

    server {
        listen %s;
        proxy_pass dockman_upstream;
    }
}
`, upstream, port, port)
	return os.WriteFile(path, []byte(config), 0o644)
}

func printManagedDryRun(action string, rt managedRuntime, opts managedRuntimeOptions) error {
	lines := []string{}
	add := func(args ...string) {
		lines = append(lines, fmt.Sprintf("Dry run: %s", formatCommand(append([]string{"docker"}, args...))))
	}

	switch action {
	case "start":
		if rt.ZeroDowntime {
			lines = append(lines, fmt.Sprintf("Dry run: adoptable container names %s", strings.Join(adoptableNames(rt), ", ")))
			add("network", "inspect", rt.NetworkName)
			add("network", "create", rt.NetworkName)
			add("image", "inspect", "--format", "{{if .Config.Healthcheck}}present{{else}}missing{{end}}", rt.Image)
			add(managedRunArgs(rt, fmt.Sprintf("%s-<revision>", sanitizeResourceName(rt.Name)), true)...)
			lines = append(lines, fmt.Sprintf("Dry run: wait for %s-<revision> to become healthy", sanitizeResourceName(rt.Name)))
			add("run", "-d", "--name", rt.ProxyName, "--label", dockmanManagedLabel, "--label", fmt.Sprintf("%s=%s", dockmanAppLabelKey, rt.Name), "--label", fmt.Sprintf("%s=%s", dockmanRoleLabelKey, dockmanRoleProxy), "--network", rt.NetworkName, "-p", rt.PublishSpec, "-v", fmt.Sprintf("%s:/etc/nginx/nginx.conf:ro", rt.ProxyConfigPath), proxyImage)
		} else {
			lines = append(lines, fmt.Sprintf("Dry run: adoptable container names %s", strings.Join(adoptableNames(rt), ", ")))
			if rt.HostPort != "" {
				add("ps", "--filter", fmt.Sprintf("publish=%s", rt.HostPort), "--format", "{{.Names}}")
			}
			add(managedRunArgs(rt, rt.Name, false)...)
		}
	case "stop":
		add("ps", "-aq", "--filter", fmt.Sprintf("label=%s", dockmanManagedLabel), "--filter", fmt.Sprintf("label=%s=%s", dockmanAppLabelKey, rt.Name), "--filter", fmt.Sprintf("label=%s=%s", dockmanRoleLabelKey, dockmanRoleApp))
		add("ps", "-aq", "--filter", fmt.Sprintf("label=%s", dockmanManagedLabel), "--filter", fmt.Sprintf("label=%s=%s", dockmanAppLabelKey, rt.Name), "--filter", fmt.Sprintf("label=%s=%s", dockmanRoleLabelKey, dockmanRoleProxy))
	case "restart", "upgrade":
		if action == "upgrade" {
			add("pull", rt.Image)
		}
		if rt.ZeroDowntime {
			lines = append(lines, fmt.Sprintf("Dry run: adoptable container names %s", strings.Join(adoptableNames(rt), ", ")))
			add("network", "inspect", rt.NetworkName)
			add("network", "create", rt.NetworkName)
			add("image", "inspect", "--format", "{{if .Config.Healthcheck}}present{{else}}missing{{end}}", rt.Image)
			add(managedRunArgs(rt, fmt.Sprintf("%s-<revision>", sanitizeResourceName(rt.Name)), true)...)
			lines = append(lines, fmt.Sprintf("Dry run: wait for %s-<revision> to become healthy", sanitizeResourceName(rt.Name)))
			lines = append(lines, fmt.Sprintf("Dry run: write proxy config %s for upstream %s-<revision>:%s", rt.ProxyConfigPath, sanitizeResourceName(rt.Name), rt.ListenPort))
			add("run", "-d", "--name", rt.ProxyName, "--label", dockmanManagedLabel, "--label", fmt.Sprintf("%s=%s", dockmanAppLabelKey, rt.Name), "--label", fmt.Sprintf("%s=%s", dockmanRoleLabelKey, dockmanRoleProxy), "--network", rt.NetworkName, "-p", rt.PublishSpec, "-v", fmt.Sprintf("%s:/etc/nginx/nginx.conf:ro", rt.ProxyConfigPath), proxyImage)
			add("exec", rt.ProxyName, "nginx", "-s", "reload")
			add("rm", "-f", "<old-managed-app-container>")
		} else {
			lines = append(lines, fmt.Sprintf("Dry run: adoptable container names %s", strings.Join(adoptableNames(rt), ", ")))
			add("ps", "-aq", "--filter", fmt.Sprintf("label=%s", dockmanManagedLabel), "--filter", fmt.Sprintf("label=%s=%s", dockmanAppLabelKey, rt.Name), "--filter", fmt.Sprintf("label=%s=%s", dockmanRoleLabelKey, dockmanRoleApp))
			for _, name := range adoptableNames(rt) {
				add("ps", "-aq", "--filter", fmt.Sprintf("name=^%s$", name))
				add("rm", "-f", name)
			}
			if rt.HostPort != "" {
				add("ps", "--filter", fmt.Sprintf("publish=%s", rt.HostPort), "--format", "{{.Names}}")
			}
			add(managedRunArgs(rt, rt.Name, false)...)
		}
	default:
		return fmt.Errorf("unknown lifecycle action: %s", action)
	}

	fmt.Println(strings.Join(lines, "\n"))
	return nil
}

func runDockerCommand(args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runDockerOutput(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if strings.TrimSpace(stderr.String()) != "" {
			return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}

func sanitizeResourceName(input string) string {
	replacer := strings.NewReplacer("/", "-", ":", "-", " ", "-")
	return replacer.Replace(input)
}
