package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var interpolationPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

type envAssignment struct {
	Key       string
	Value     string
	Sensitive bool
}

func interpolateString(input string) (string, error) {
	matches := interpolationPattern.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input, nil
	}

	missing := make([]string, 0)
	seenMissing := make(map[string]bool)
	resolved := interpolationPattern.ReplaceAllStringFunc(input, func(match string) string {
		name := interpolationPattern.FindStringSubmatch(match)[1]
		value, ok := os.LookupEnv(name)
		if !ok {
			if !seenMissing[name] {
				missing = append(missing, name)
				seenMissing[name] = true
			}
			return ""
		}
		return value
	})

	if len(missing) > 0 {
		sort.Strings(missing)
		return "", fmt.Errorf("missing variables: %s", strings.Join(missing, ", "))
	}

	return resolved, nil
}

func resolveStringField(label string, input string) (string, error) {
	resolved, err := interpolateString(strings.TrimSpace(input))
	if err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return resolved, nil
}

func resolveStringMap(label string, input map[string]string, sensitive bool) ([]envAssignment, error) {
	if len(input) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	assignments := make([]envAssignment, 0, len(keys))
	for _, key := range keys {
		value, err := resolveStringField(fmt.Sprintf("%s.%s", label, key), input[key])
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, envAssignment{
			Key:       key,
			Value:     value,
			Sensitive: sensitive,
		})
	}

	return assignments, nil
}

func resolveStringSlice(label string, input []string) ([]string, error) {
	if len(input) == 0 {
		return nil, nil
	}

	resolved := make([]string, 0, len(input))
	for i, value := range input {
		item, err := resolveStringField(fmt.Sprintf("%s[%d]", label, i), value)
		if err != nil {
			return nil, err
		}
		if item != "" {
			resolved = append(resolved, item)
		}
	}
	return resolved, nil
}

func envAssignmentsToStrings(assignments []envAssignment) []string {
	if len(assignments) == 0 {
		return nil
	}

	out := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		out = append(out, fmt.Sprintf("%s=%s", assignment.Key, assignment.Value))
	}
	return out
}

func formatCommandMasked(args []string, masked map[int]bool) string {
	escaped := make([]string, 0, len(args))
	for i, arg := range args {
		if masked[i] {
			if key, _, ok := strings.Cut(arg, "="); ok {
				escaped = append(escaped, shellEscape(key+"=<redacted>"))
				continue
			}
			escaped = append(escaped, "<redacted>")
			continue
		}
		escaped = append(escaped, shellEscape(arg))
	}
	return strings.Join(escaped, " ")
}
