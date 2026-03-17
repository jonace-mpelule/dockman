package main

import (
	"fmt"
	"os"
	"os/exec"
)

const installScriptURL = "https://raw.githubusercontent.com/jonace-mpelule/dockman/main/scripts/install.sh"

var updateCommandRunner = func(dryRun bool) error {
	if dryRun {
		fmt.Printf("Dry run: curl -fsSL %s | sh\n", installScriptURL)
		return nil
	}

	cmd := exec.Command("sh", "-c", fmt.Sprintf("curl -fsSL %s | sh", installScriptURL))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func selfUpdate(dryRun bool) error {
	return updateCommandRunner(dryRun)
}
