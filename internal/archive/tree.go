package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"imgproxy_plus/internal/config"
)

type FileEntry struct {
	AbsPath  string
	RelPath  string
	Name     string
	IsAnimated bool
}

type Group struct {
	Name      string
	DirPath   string
	Images    []FileEntry
	NonImages []FileEntry
}

func BuildTree(rootPath string, cfg *config.Config) ([]*Group, error) {
	rootName := filepath.Base(rootPath)

	tn := &treeNode{
		name:    rootName,
		dirPath: rootPath,
	}

	buildTreeNode(tn, rootPath, rootName)

	flattenNode(tn, cfg)

	var groups []*Group
	collectGroups(tn, &groups)

	if len(groups) == 0 {
		return []*Group{{
			Name:    rootName,
			DirPath: rootPath,
		}}, nil
	}

	return groups, nil
}

type treeNode struct {
	name     string
	dirPath  string
	images   []FileEntry
	nonImgs  []FileEntry
	children []*treeNode
	merged   bool
}

func buildTreeNode(node *treeNode, dirPath, prefix string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	for _, e := range entries {
		name := e.Name()
		if name == ".tmp" || strings.HasPrefix(name, ".") {
			continue
		}
		absPath := filepath.Join(dirPath, name)

		if e.IsDir() {
			childName := prefix + "-" + name
			if prefix == node.name {
				childName = node.name + "-" + name
			}

			childDirPath := filepath.Join(dirPath, name)
			child := &treeNode{
				name:    ensureUniqueGroupName(node.children, childName),
				dirPath: childDirPath,
			}
			buildTreeNode(child, childDirPath, childName)
			node.children = append(node.children, child)
		} else if IsImageExt(name) {
			node.images = append(node.images, FileEntry{
				AbsPath:  absPath,
				RelPath:  name,
				Name:     name,
				IsAnimated: DetectAnimated(absPath),
			})
		} else {
			node.nonImgs = append(node.nonImgs, FileEntry{
				AbsPath: absPath,
				RelPath: name,
				Name:    name,
			})
		}
	}

	sort.Slice(node.images, func(i, j int) bool {
		return naturalCmp(node.images[i].Name, node.images[j].Name) < 0
	})
}

func ensureUniqueGroupName(siblings []*treeNode, name string) string {
	for _, s := range siblings {
		if s.name == name {
			return name + "_2"
		}
	}
	return name
}

func flattenNode(node *treeNode, cfg *config.Config) {
	for i := len(node.children) - 1; i >= 0; i-- {
		child := node.children[i]
		flattenNode(child, cfg)

		childImgCount := countAllImages(child)

		if childImgCount >= cfg.GalleryArchiveMinChapter {
			continue
		}

		for _, subChild := range child.children {
			if !subChild.merged {
				node.children = append(node.children, subChild)
			}
		}

		node.images = append(node.images, child.images...)
		node.nonImgs = append(node.nonImgs, child.nonImgs...)
		child.merged = true
	}

	active := make([]*treeNode, 0)
	for _, c := range node.children {
		if !c.merged {
			active = append(active, c)
		}
	}
	node.children = active
}

func countAllImages(node *treeNode) int {
	count := len(node.images)
	for _, child := range node.children {
		if !child.merged {
			count += countAllImages(child)
		}
	}
	return count
}

func collectGroups(node *treeNode, result *[]*Group) {
	if node.merged {
		return
	}
	if len(node.images) > 0 {
		*result = append(*result, &Group{
			Name:    node.name,
			DirPath: node.dirPath,
			Images:  node.images,
		})
	}
	for _, child := range node.children {
		collectGroups(child, result)
	}
}

func CountImagesRecursive(dirPath string) int {
	count := 0
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.Name() == ".tmp" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			count += CountImagesRecursive(filepath.Join(dirPath, e.Name()))
		} else if IsImageExt(e.Name()) {
			count++
		}
	}
	return count
}

func HasAnyContent(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() == ".tmp" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		return true
	}
	return false
}

func HasMediaFiles(dirPath string) bool {
	return hasMediaRecursive(dirPath)
}

func hasMediaRecursive(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() == ".tmp" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			if hasMediaRecursive(filepath.Join(dirPath, e.Name())) {
				return true
			}
		} else if IsMediaExt(e.Name()) {
			return true
		}
	}
	return false
}

func naturalCmp(a, b string) int {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	re := func(r rune) bool { return r < '0' || r > '9' }
	fa := strings.FieldsFunc(la, re)
	fb := strings.FieldsFunc(lb, re)
	for i := 0; i < len(fa) && i < len(fb); i++ {
		na := atoi(fa[i])
		nb := atoi(fb[i])
		if na != nb {
			return na - nb
		}
		cmp := strings.Compare(fa[i], fb[i])
		if cmp != 0 {
			return cmp
		}
	}
	if len(fa) != len(fb) {
		return len(fa) - len(fb)
	}
	return strings.Compare(la, lb)
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func sortByNatural(entries []FileEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return naturalCmp(entries[i].Name, entries[j].Name) < 0
	})
}

func selectCover(images []FileEntry) (*FileEntry, int) {
	if len(images) == 0 {
		return nil, -1
	}

	for i, img := range images {
		if HasCoverWord(img.Name) {
			return &images[i], i
		}
	}

	return &images[0], 0
}

func OpenProcessing(path string) (*os.File, error) {
	lockPath := filepath.Join(path, ".gallery_processing")
	return os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
}

func MarkFailed(oldPath string, reason error) error {
	newPath := oldPath + "_failed"
	errPath := filepath.Join(oldPath, ".gallery_error")
	os.WriteFile(errPath, []byte(fmt.Sprintf("%v", reason)), 0644)
	newPathClean := filepath.Clean(newPath)
	oldPathClean := filepath.Clean(oldPath)
	if oldPathClean == newPathClean {
		return nil
	}
	return os.Rename(oldPath, newPath)
}

func CleanupDir(dirPath string) {
	os.Remove(filepath.Join(dirPath, ".gallery_processing"))
	os.Remove(filepath.Join(dirPath, ".gallery_error"))
	os.RemoveAll(filepath.Join(dirPath, ".tmp"))
}
