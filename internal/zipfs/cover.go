package zipfs

import (
	"archive/zip"
	"path/filepath"
	"sort"
	"strings"

	"imgproxy_plus/internal/ziputil"
)

// isImageExt reports whether name looks like a raster image supported by the
// gallery (used for cover fallback selection inside a cbz).
func isImageExt(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".jfif", ".png", ".gif", ".webp", ".avif", ".bmp":
		return true
	}
	return false
}

// coverRank returns a sort priority for a cover candidate. Lower rank = higher
// priority. Non-cover images get a large rank so they sort last.
//
// Priority order:
//  1. "__cover.*"   (double-underscore builtin cover, e.g. __cover.jfif)
//  2. "##cover.*"   (double-hash marker, e.g. ##cover.jfif)
//  3. any file whose stem contains "cover" (e.g. 00_cover.jpg)
//  4. otherwise ranked by natural name so the first image wins
func coverRank(name string) (rank int, isCover bool) {
	base := strings.ToLower(filepath.Base(name))
	stem := strings.TrimSuffix(base, strings.ToLower(filepath.Ext(base)))

	switch {
	case strings.HasPrefix(base, "__cover"):
		return 0, true
	case strings.HasPrefix(base, "##cover"):
		return 1, true
	case strings.Contains(stem, "cover"):
		return 2, true
	}
	return 1 << 30, false
}

// pickCoverFile selects the best cover entry from a cbz according to the
// priority order. It returns nil when the archive has no image entries.
// Ties within the same priority bucket are broken by natural name order.
func pickCoverFile(files []*zip.File) *zip.File {
	type cand struct {
		f    *zip.File
		rank int
		name string
	}
	var cands []cand
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}
		name := ziputil.DecodeName(f)
		if !isImageExt(name) {
			continue
		}
		// ignore entries nested in subdirectories — covers live at cbz root
		if strings.Contains(strings.TrimSuffix(name, "/"), "/") {
			continue
		}
		rank, _ := coverRank(name)
		cands = append(cands, cand{f: f, rank: rank, name: name})
	}
	if len(cands) == 0 {
		return nil
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].rank != cands[j].rank {
			return cands[i].rank < cands[j].rank
		}
		return naturalLess(cands[i].name, cands[j].name)
	})
	return cands[0].f
}

// naturalLess compares strings with embedded numbers compared numerically.
func naturalLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	ai, bi := 0, 0
	for ai < len(la) && bi < len(lb) {
		ca, cb := la[ai], lb[bi]
		if isDigit(ca) && isDigit(cb) {
			na, ea := readNum(la[ai:])
			nb, eb := readNum(lb[bi:])
			if na != nb {
				return na < nb
			}
			ai += ea
			bi += eb
			continue
		}
		if ca != cb {
			return ca < cb
		}
		ai++
		bi++
	}
	return len(la)-ai < len(lb)-bi
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func readNum(s string) (n, count int) {
	for count < len(s) && isDigit(s[count]) {
		n = n*10 + int(s[count]-'0')
		count++
	}
	return
}
