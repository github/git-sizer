package main_test

import (
	"os/exec"
	"testing"
)

// Smoke test that the program runs.
func TestExec(t *testing.T) {
	command := exec.Command("bin/git-sizer", ".")
	command.Dir = "../.."
	output, err := command.CombinedOutput()
	if err != nil {
		t.Errorf("command failed (%s); output: %#v", err, string(output))
	}
}
