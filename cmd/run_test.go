package cmd

import (
	"testing"
)

func TestRunCmd_RequiresAgentArg(t *testing.T) {
	cmd := rootCmd
	cmd.SetArgs([]string{"run"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when agent arg not provided")
	}
}

func TestRunCmd_RequiresGoalFlag(t *testing.T) {
	cmd := rootCmd
	cmd.SetArgs([]string{"run", "build-feature"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --goal not provided")
	}
}
