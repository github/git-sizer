package git

import (
	"bytes"
	"fmt"
	"strconv"
)

func (repo *Repository) ConfigStringDefault(key string, defaultValue string) (string, error) {
	cmd := repo.gitCommand(
		"config",
		"--default", defaultValue,
		key,
	)

	out, err := cmd.Output()
	if err != nil {
		return defaultValue, fmt.Errorf("running 'git config': %w", err)
	}

	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}

	return string(out), nil
}

func (repo *Repository) ConfigBoolDefault(key string, defaultValue bool) (bool, error) {
	cmd := repo.gitCommand(
		"config",
		"--type", "bool",
		"--default", strconv.FormatBool(defaultValue),
		key,
	)

	out, err := cmd.Output()
	if err != nil {
		return defaultValue, fmt.Errorf("running 'git config': %w", err)
	}

	s := string(bytes.TrimSpace(out))
	value, err := strconv.ParseBool(s)
	if err != nil {
		return defaultValue, fmt.Errorf("unexpected bool value from 'git config': %q", s)
	}

	return value, nil
}

func (repo *Repository) ConfigIntDefault(key string, defaultValue int) (int, error) {
	cmd := repo.gitCommand(
		"config",
		"--type", "int",
		"--default", strconv.Itoa(defaultValue),
		key,
	)

	out, err := cmd.Output()
	if err != nil {
		return defaultValue, fmt.Errorf("running 'git config': %w", err)
	}

	s := string(bytes.TrimSpace(out))
	value, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue, fmt.Errorf("unexpected int value from 'git config': %q", s)
	}

	return value, nil
}
