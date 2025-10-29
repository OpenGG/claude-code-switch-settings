package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMainRuns(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("CCS_BASE_DIR", filepath.Join(tempDir, ".claude"))
	os.Setenv("CCS_NON_INTERACTIVE", "1")
	defer os.Unsetenv("CCS_BASE_DIR")
	defer os.Unsetenv("CCS_NON_INTERACTIVE")

	main()
}
