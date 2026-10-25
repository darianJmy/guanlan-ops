package archiveutil

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ExtractTarGzip(data []byte, destination string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return errors.New("Git 制品格式不正确")
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("解压 Git 制品失败")
		}
		cleaned := filepath.Clean(filepath.FromSlash(header.Name))
		if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return errors.New("Git 制品包含非法路径")
		}
		target := filepath.Join(destination, cleaned)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0777)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, tarReader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			link := filepath.Clean(filepath.FromSlash(header.Linkname))
			if filepath.IsAbs(link) || link == ".." || strings.HasPrefix(link, ".."+string(filepath.Separator)) {
				return errors.New("Git 制品包含非法符号链接")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		default:
			return errors.New("Git 制品包含不支持的文件类型")
		}
	}
}
