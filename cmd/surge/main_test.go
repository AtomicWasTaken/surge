package main

import (
	"bytes"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitAndMainSuccess(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"surge", "--help"}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	main()

	require.NoError(t, w.Close())
	os.Stdout = stdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())
	assert.Contains(t, buf.String(), "surge is an AI-powered code review tool for pull requests.")
}

func TestMainErrorExit(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMainErrorHelper", "--", "unknown")
	cmd.Env = append(os.Environ(), "SURGE_MAIN_ERROR_HELPER=1")
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(output), "error: unknown command")
}

func TestMainErrorHelper(t *testing.T) {
	if os.Getenv("SURGE_MAIN_ERROR_HELPER") != "1" {
		return
	}
	os.Args = []string{"surge", "unknown"}
	main()
}
