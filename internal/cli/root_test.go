package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetVersion(t *testing.T) {
	SetVersion("1.2.3", "abc", "2026-03-20")
	assert.Equal(t, "1.2.3", versionInfo.Version)
	assert.Equal(t, "1.2.3", rootCmd.Version)
}

func TestRunConfigInitValidateDiffAndHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	prevWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	require.NoError(t, runConfigInit(rootCmd, nil))
	err = runConfigInit(rootCmd, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "surge.yaml already exists")

	prevConfig := flagConfig
	flagConfig = filepath.Join(tmpDir, "surge.yaml")
	t.Cleanup(func() { flagConfig = prevConfig })

	require.NoError(t, runConfigValidate(rootCmd, nil))
	require.NoError(t, runDiff(rootCmd, nil))

	require.NoError(t, w.Close())
	os.Stdout = stdout

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	output := buf.String()
	assert.Contains(t, output, "Created surge.yaml")
	assert.Contains(t, output, "Config is valid")
	assert.Contains(t, output, "Diff command not yet implemented")
	resolvedTmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, resolvedTmpDir, cwd())
}

func TestRunConfigInitWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	prevWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(prevWD) })
	require.NoError(t, os.Chmod(tmpDir, 0500))
	t.Cleanup(func() { _ = os.Chmod(tmpDir, 0755) })

	err = runConfigInit(rootCmd, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to write config")
}

func TestDetectGitInfo(t *testing.T) {
	t.Run("from git config", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		require.NoError(t, os.Mkdir(gitDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("[remote \"origin\"]\n\turl = https://github.com/octo/surge.git\n"), 0644))

		prevWD, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(tmpDir))
		t.Cleanup(func() { _ = os.Chdir(prevWD) })

		owner, repo, err := detectGitInfo()
		require.NoError(t, err)
		assert.Equal(t, "octo", owner)
		assert.Equal(t, "surge", repo)
	})

	t.Run("from ssh git config", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		require.NoError(t, os.Mkdir(gitDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("[remote \"origin\"]\n\turl = git@github.com:octo/surge.git\n"), 0644))

		prevWD, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(tmpDir))
		t.Cleanup(func() { _ = os.Chdir(prevWD) })

		owner, repo, err := detectGitInfo()
		require.NoError(t, err)
		assert.Equal(t, "octo", owner)
		assert.Equal(t, "surge", repo)
	})

	t.Run("from env", func(t *testing.T) {
		tmpDir := t.TempDir()
		prevWD, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(tmpDir))
		t.Cleanup(func() { _ = os.Chdir(prevWD) })

		t.Setenv("GITHUB_REPOSITORY", "env-owner/env-repo")

		owner, repo, err := detectGitInfo()
		require.NoError(t, err)
		assert.Equal(t, "env-owner", owner)
		assert.Equal(t, "env-repo", repo)
	})

	t.Run("error", func(t *testing.T) {
		tmpDir := t.TempDir()
		prevWD, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(tmpDir))
		t.Cleanup(func() { _ = os.Chdir(prevWD) })

		t.Setenv("GITHUB_REPOSITORY", "invalid")

		_, _, err = detectGitInfo()
		require.Error(t, err)
		assert.ErrorContains(t, err, "could not detect git owner/repo")
	})
}

func TestRunDiffConfigError(t *testing.T) {
	prevConfig := flagConfig
	flagConfig = filepath.Join(t.TempDir(), "missing.yaml")
	t.Cleanup(func() { flagConfig = prevConfig })

	err := runDiff(rootCmd, nil)
	require.Error(t, err)
}

func TestDetectGitInfoGetwdError(t *testing.T) {
	prev := getwd
	getwd = func() (string, error) { return "", errors.New("boom") }
	t.Cleanup(func() { getwd = prev })

	_, _, err := detectGitInfo()
	require.Error(t, err)
	assert.ErrorContains(t, err, "boom")
}

func TestExecute(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	rootCmd.SetArgs([]string{"--help"})
	require.NoError(t, Execute())
}

func TestRunConfigValidateError(t *testing.T) {
	prevConfig := flagConfig
	flagConfig = filepath.Join(t.TempDir(), "missing.yaml")
	t.Cleanup(func() { flagConfig = prevConfig })

	err := runConfigValidate(rootCmd, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "config validation failed")
}

func TestRunConfigValidateInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "surge.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
ai:
  provider: invalid
output:
  format: terminal
contextDepth: diff-only
categories:
  security: true
`), 0644))

	prevConfig := flagConfig
	flagConfig = cfgPath
	t.Cleanup(func() { flagConfig = prevConfig })

	err := runConfigValidate(rootCmd, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "config is invalid")
}
