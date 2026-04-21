package testsupport

import (
	"os"
	"path/filepath"
)

// init centralizes Allure output in the repository root instead of per-package folders.
func init() {
	if os.Getenv("ALLURE_OUTPUT_PATH") != "" {
		return
	}

	root, err := findRepositoryRoot()
	if err != nil {
		return
	}
	_ = os.Setenv("ALLURE_OUTPUT_PATH", root)
}

// findRepositoryRoot walks up from the current test package directory to the module root.
func findRepositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
