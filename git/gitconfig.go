package git

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type ConfigEntry struct {
	Key   string
	Value string
}

type Config struct {
	Entries []ConfigEntry
}

// Config returns the entries from gitconfig. If `prefix` is provided,
// then only include entries in that section, which must match the at
// a component boundary (as defined by `configKeyMatchesPrefix()`),
// and strip off the prefix in the keys that are returned.
func (repo *Repository) Config(prefix string) (*Config, error) {
	cmd := repo.gitCommand("config", "--list", "-z")

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading git configuration: %w", err)
	}

	var config Config

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

// configKeyMatchesPrefix checks whether `key` starts with `prefix` at
// a component boundary (i.e., at a '.'). If yes, it returns `true`
// and the part of the key after the prefix; e.g.:
//
//     configKeyMatchesPrefix("foo.bar", "foo") → true, "bar"
//     configKeyMatchesPrefix("foo.bar", "foo.") → true, "bar"
//     configKeyMatchesPrefix("foo.bar", "foo.bar") → true, ""
//     configKeyMatchesPrefix("foo.bar", "foo.bar.") → false, ""
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
