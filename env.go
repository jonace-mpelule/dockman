package main

import (
	"bufio"
	"os"
	"strconv"
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
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}

		value := strings.TrimSpace(parts[1])
		if value == "" {
			env[key] = value
			continue
		}

		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
			unquoted, err := strconv.Unquote(value)
			if err == nil {
				value = unquoted
			} else {
				value = strings.Trim(value, "\"")
			}
		} else if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2 {
			value = strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
		} else {
			value = stripInlineComment(value)
		}

		env[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return env, nil
}

func stripInlineComment(value string) string {
	seenSpace := false
	for i, r := range value {
		if r == '#' && seenSpace {
			return strings.TrimSpace(value[:i])
		}
		if r == ' ' || r == '\t' {
			seenSpace = true
		} else {
			seenSpace = false
		}
	}
	return strings.TrimSpace(value)
}
