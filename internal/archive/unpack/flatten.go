package unpack

import "os"

// FlattenSingleWrapper recursively flattens chains of single-subdir directories.
// After unpacking, the archive name is preserved as a top-level directory,
// but if the archive itself was a single-dir wrapper (e.g. archive.zip
// containing only archive/ subdir), we collapse those redundant levels.
func FlattenSingleWrapper(dirPath string) {
	for {
		entries, err := os.ReadDir(dirPath)
		if err != nil || len(entries) != 1 {
			break
		}
		if !entries[0].IsDir() {
			break
		}
		subPath := dirPath + "/" + entries[0].Name()
		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			break
		}
		for _, se := range subEntries {
			os.Rename(subPath+"/"+se.Name(), dirPath+"/"+se.Name())
		}
		os.Remove(subPath)
	}
}
