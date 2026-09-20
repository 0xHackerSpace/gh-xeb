package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestExecuteToolCommand(t *testing.T) {
	root := NewRootCmd(DefaultDeps())
	stdout, stderr, err := executeToolCommand(context.Background(), root, "doctor", []string{})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if stdout == "" {
		t.Errorf("expected non-empty stdout, got empty string")
	}

	if !strings.Contains(stdout, "GitHub") {
		t.Errorf("expected 'GitHub' in output, got:\n%s", stdout)
	}

	t.Logf("stderr: %s", stderr)
	t.Logf("stdout length: %d", len(stdout))
}
