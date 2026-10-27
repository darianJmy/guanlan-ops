package archiveutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func archive(t *testing.T, headers []*tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, h := range headers {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write(bytes.Repeat([]byte("x"), int(h.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestExtractModesAndRelativeSymlinks(t *testing.T) {
	data := archive(t, []*tar.Header{{Name: "bin/run", Typeflag: tar.TypeReg, Size: 3, Mode: 0755}, {Name: "launcher", Typeflag: tar.TypeSymlink, Linkname: "bin/run", Mode: 0777}})
	root := t.TempDir()
	if err := ExtractTarGzip(data, root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "launcher"))
	if err != nil || string(got) != "xxx" {
		t.Fatalf("content: %q %v", got, err)
	}
	info, err := os.Stat(filepath.Join(root, "bin/run"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatal("executable mode lost")
	}
}
func TestExtractRejectsEscapesAndUnsupportedEntries(t *testing.T) {
	for _, h := range []*tar.Header{{Name: "../outside", Typeflag: tar.TypeReg}, {Name: "/absolute", Typeflag: tar.TypeReg}, {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"}, {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/absolute"}, {Name: "device", Typeflag: tar.TypeChar}} {
		if err := ExtractTarGzip(archive(t, []*tar.Header{h}), t.TempDir()); err == nil {
			t.Errorf("accepted unsafe entry: %+v", h)
		}
	}
	if err := ExtractTarGzip([]byte("bad archive"), t.TempDir()); err == nil {
		t.Fatal("accepted invalid gzip")
	}
}
