package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/asksurf-ai/surf-cli/cli"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"
)

func initTestCLI(t *testing.T) {
	t.Helper()
	viper.Reset()
	viper.Set("nocolor", true)
	viper.Set("tty", true)
	viper.Set("rsh-retry", 0)
	viper.Set("rsh-profile", "default")

	cli.Init("test", "1.0.0")
	cli.Defaults()

	// Use a temp dir for config.
	configDir = t.TempDir()
}

// captureStdout runs fn and returns whatever it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	assert.NoError(t, err)

	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

// captureStderr runs fn and returns whatever it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	assert.NoError(t, err)

	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestAuthSaveToKeychain(t *testing.T) {
	initTestCLI(t)
	keyring.MockInit()

	cmd := newAuthCmd()
	cmd.SetArgs([]string{"--api-key", "sk-test-key-12345"})

	out := captureStderr(t, func() {
		err := cmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "API key saved")

	// Verify it's in the keychain.
	token, err := keyring.Get(cli.KeyringService, "surf:default")
	assert.NoError(t, err)
	assert.Equal(t, "sk-test-key-12345", token)
}

func TestAuthClear(t *testing.T) {
	initTestCLI(t)
	keyring.MockInit()
	keyring.Set(cli.KeyringService, "surf:default", "sk-to-be-cleared")

	cmd := newAuthCmd()
	cmd.SetArgs([]string{"--clear"})

	out := captureStderr(t, func() {
		err := cmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "API key cleared")

	// Verify keychain is empty.
	token, err := keyring.Get(cli.KeyringService, "surf:default")
	assert.Error(t, err)
	assert.Empty(t, token)
}

func TestAuthStatusFromEnv(t *testing.T) {
	initTestCLI(t)
	t.Setenv("SURF_API_KEY", "sk-env-key-67890")

	cmd := newAuthCmd()
	cmd.SetArgs([]string{})

	out := captureStdout(t, func() {
		err := cmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "SURF_API_KEY")
	assert.Contains(t, out, "env")
}

func TestAuthStatusFromKeychain(t *testing.T) {
	initTestCLI(t)
	t.Setenv("SURF_API_KEY", "")
	keyring.MockInit()
	keyring.Set(cli.KeyringService, "surf:default", "sk-keychain-key-111")

	cmd := newAuthCmd()
	cmd.SetArgs([]string{})

	out := captureStdout(t, func() {
		err := cmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "keychain")
}

func TestAuthStatusNoKey(t *testing.T) {
	initTestCLI(t)
	t.Setenv("SURF_API_KEY", "")
	keyring.MockInit()

	cmd := newAuthCmd()
	cmd.SetArgs([]string{})

	out := captureStdout(t, func() {
		err := cmd.Execute()
		assert.NoError(t, err)
	})
	assert.Contains(t, out, "No API key configured")
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sk-abc", "*****"},
		{"sk-12345678901234", "sk-1**********1234"},
	}
	for _, tt := range tests {
		got := maskKey(tt.input)
		if len(tt.input) <= 8 {
			assert.Equal(t, strings.Repeat("*", len(tt.input)), got)
		} else {
			assert.True(t, strings.HasPrefix(got, tt.input[:4]), "should start with first 4 chars")
			assert.True(t, strings.HasSuffix(got, tt.input[len(tt.input)-4:]), "should end with last 4 chars")
			assert.Equal(t, len(tt.input), len(got), "masked key should be same length")
		}
	}
}
