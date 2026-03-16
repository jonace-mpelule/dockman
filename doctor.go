package main

import (
	"bytes"
	"fmt"
	"os"
)

func doctorConfig(path string, dryRun bool) error {
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	updated, err := marshalConfig(cfg)
	if err != nil {
		return err
	}

	if bytes.Equal(updated, original) {
		fmt.Printf("Config is up to date: %s\n", path)
		return nil
	}

	if dryRun {
		fmt.Printf("Config needs updates: %s\n", path)
		return nil
	}

	backupPath := path + ".bak"
	if err := os.WriteFile(backupPath, original, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return err
	}

	fmt.Printf("Updated config: %s\nBackup written to: %s\n", path, backupPath)
	return nil
}
