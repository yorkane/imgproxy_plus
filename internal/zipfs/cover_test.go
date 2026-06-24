package zipfs

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"io"
	"strings"
	"testing"
)

func makeZip(t *testing.T, names []string) []*zip.File {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("x"))
	}
	zw.Close()
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return zr.File
}

func TestPickCover(t *testing.T) {
	cases := []struct {
		name   string
		files  []string
		expect string
	}{
		{"builtin __cover wins", []string{"003_page.png", "002_cover.png", "__cover.jfif", "001_page.png"}, "__cover.jfif"},
		{"##cover second", []string{"001_page.png", "010_cover.png", "##cover.jfif"}, "##cover.jfif"},
		{"cover word -> natural first among cover-named", []string{"005_page.png", "001_page.png", "cover_front.png", "003_cover.png"}, "003_cover.png"},
		{"no cover -> natural first", []string{"010_b.png", "002_a.png", "005_c.png"}, "002_a.png"},
		{"natural within cover bucket", []string{"009_cover.png", "001_cover.png", "005_page.png"}, "001_cover.png"},
		{"empty archive", []string{"a/"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := makeZip(t, c.files)
			got := pickCoverFile(files)
			if c.expect == "" {
				if got != nil {
					t.Fatalf("expected nil, got %s", got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected %s, got nil", c.expect)
			}
			if !strings.HasSuffix(got.Name, c.expect) {
				t.Fatalf("expected %s, got %s", c.expect, got.Name)
			}
		})
	}
}

func TestIsCoverRequest(t *testing.T) {
	cases := map[string]bool{
		"__cover.jfif":  true,
		"##cover.jfif":  true,
		"cover.jpg":     true,
		"005_cover.png": true,
		"001_page.png":  false,
		"readme.txt":    false,
	}
	for path, want := range cases {
		if got := isCoverRequest(path); got != want {
			t.Errorf("isCoverRequest(%q) = %v, want %v", path, got, want)
		}
	}
}
