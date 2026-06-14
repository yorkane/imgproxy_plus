package unpack

import (
	"fmt"
	"os"
	"os/exec"
)

func UnpackRar(archivePath, destDir string) error {
	os.MkdirAll(destDir, 0755)
	cmd := exec.Command("unrar", "x", "-o+", archivePath, destDir+"/")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unrar: %s: %w", string(out), err)
	}
	return nil
}
