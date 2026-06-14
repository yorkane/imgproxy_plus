package unpack

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Unpack7z(archivePath, destDir string) error {
	os.MkdirAll(destDir, 0755)
	absDest, _ := filepath.Abs(destDir)
	cmd := exec.Command("7z", "x", "-y", "-o"+absDest, archivePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z: %s: %w", string(out), err)
	}
	return nil
}
