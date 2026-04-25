// Package appmode stores supported application startup modes.
package appmode

import (
	"fmt"
	"os"
	"strings"
)

// Mode identifies which application variant should be started.
type Mode string

const (
	// EnvVar stores the environment variable used to select the startup mode.
	EnvVar = "LOGMONITOR_APP_MODE"

	// CLI starts the standalone console utility.
	CLI Mode = "CLI"
	// HTTP starts the long-running HTTP server process.
	HTTP Mode = "HTTP"
)

// Parse converts a raw string into a supported startup mode.
func Parse(value string) (Mode, error) {
	mode := Mode(strings.ToUpper(strings.TrimSpace(value)))
	switch mode {
	case CLI, HTTP:
		return mode, nil
	default:
		return "", fmt.Errorf("%s must be one of %q or %q", EnvVar, CLI, HTTP)
	}
}

// FromEnv reads and validates the startup mode from the environment.
func FromEnv() (Mode, error) {
	value := strings.TrimSpace(os.Getenv(EnvVar))
	if value == "" {
		return "", fmt.Errorf("%s is required", EnvVar)
	}
	return Parse(value)
}

// Require ensures that the current process was started with the expected mode.
func Require(expected Mode) (Mode, error) {
	mode, err := FromEnv()
	if err != nil {
		return "", err
	}
	if mode != expected {
		return "", fmt.Errorf("%s=%q does not match this binary mode %q", EnvVar, mode, expected)
	}
	return mode, nil
}
