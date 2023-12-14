package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ConfigEntry represents an entry in the gitconfig.
type ConfigEntry struct {
	// Key is the entry's key, with any common `prefix` removed (see
	// `Config()`).
	Key string

	// Value is the entry's value, as a string.
	Value string
}

// Config represents the gitconfig, or part of the gitconfig, read by
// `ReadConfig()`.
type Config struct {
	// Prefix is the key prefix that was read to fill this `Config`.
	Prefix string

	// Entries contains the configuration entries that matched
	// `Prefix`, in the order that they are reported by `git config
	// --list`.
	Entries []ConfigEntry
}

// GetConfig returns the entries from gitconfig. If `prefix` is
// provided, then only include entries in that section, which must
// match the at a component boundary (as defined by
// `configKeyMatchesPrefix()`), and strip off the prefix in the keys
// that are returned.
func (repo *Repository) GetConfig(prefix string) (*Config, error) {
	cmd := repo.GitCommand("config", "--list", "-z")

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading git configuration: %w", err)
	}

	config := Config{
		Prefix: prefix,
	}

	for len(out) > 0 {
		keyEnd := bytes.IndexByte(out, '\n')
		if keyEnd == -1 {
			return nil, errors.New("invalid output from 'git config'")
		}
		key := string(out[:keyEnd])
		out = out[keyEnd+1:]
		valueEnd := bytes.IndexByte(out, 0)
		if valueEnd == -1 {
			return nil, errors.New("invalid output from 'git config'")
		}
		value := string(out[:valueEnd])
		out = out[valueEnd+1:]

		ok, rest := configKeyMatchesPrefix(key, prefix)
		if !ok {
			continue
		}

		entry := ConfigEntry{
			Key:   rest,
			Value: value,
		}
		config.Entries = append(config.Entries, entry)
	}

	return &config, nil
}

// FullKey returns the full gitconfig key name for the relative key
// name `key`.
func (config *Config) FullKey(key string) string {
	if config.Prefix == "" {
		return key
	}
	return fmt.Sprintf("%s.%s", config.Prefix, key)
}

// configKeyMatchesPrefix checks whether `key` starts with `prefix` at
// a component boundary (i.e., at a '.'). If yes, it returns `true`
// and the part of the key after the prefix; e.g.:
//
//	configKeyMatchesPrefix("foo.bar", "foo") → true, "bar"
//	configKeyMatchesPrefix("foo.bar", "foo.") → true, "bar"
//	configKeyMatchesPrefix("foo.bar", "foo.bar") → true, ""
//	configKeyMatchesPrefix("foo.bar", "foo.bar.") → false, ""
func configKeyMatchesPrefix(key, prefix string) (bool, string) {
	if prefix == "" {
		return true, key
	}
	if !strings.HasPrefix(key, prefix) {
		return false, ""
	}

	if prefix[len(prefix)-1] == '.' {
		return true, key[len(prefix):]
	}
	if len(key) == len(prefix) {
		return true, ""
	}
	if key[len(prefix)] == '.' {
		return true, key[len(prefix)+1:]
	}
	return false, ""
}

func (repo *Repository) ConfigStringDefault(key string, defaultValue string) (string, error) {
	// Note that `git config --get` didn't get `--default` until Git
	// 2.18 (released 2018-06-21).
	cmd := repo.GitCommand(
		"config", "--get", key,
	)

	out, err := cmd.Output()
	if err != nil {
		if err, ok := err.(*exec.ExitError); ok && err.ExitCode() == 1 {
			// This indicates that the value was not found.
			return defaultValue, nil
		}
		return defaultValue, fmt.Errorf("running 'git config': %w", err)
	}

	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}

	return string(out), nil
}

func (repo *Repository) ConfigBoolDefault(key string, defaultValue bool) (bool, error) {
	// Note that `git config --get` didn't get `--type=bool` or
	// `--default` until Git 2.18 (released 2018-06-21).
	cmd := repo.GitCommand(
		"config", "--get", "--bool", key,
	)

	out, err := cmd.Output()
	if err != nil {
		if err, ok := err.(*exec.ExitError); ok && err.ExitCode() == 1 {
			// This indicates that the value was not found.
			return defaultValue, nil
		}
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
	// Note that `git config --get` didn't get `--type=int` or
	// `--default` until Git 2.18 (released 2018-06-21).
	cmd := repo.GitCommand(
		"config", "--get", "--int", key,
	)

	out, err := cmd.Output()
	if err != nil {
		if err, ok := err.(*exec.ExitError); ok && err.ExitCode() == 1 {
			// This indicates that the value was not found.
			return defaultValue, nil
		}
		return defaultValue, fmt.Errorf("running 'git config': %w", err)
	}

	s := string(bytes.TrimSpace(out))
	value, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue, fmt.Errorf("unexpected int value from 'git config': %q", s)
	}

	return value, nil
}
