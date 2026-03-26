package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
)

func zipDirectory(dirPath string) ([]byte, error) {
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)

	base := filepath.Base(dirPath)

	err := filepath.WalkDir(dirPath, func(pathname string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dirPath, pathname)
		if err != nil {
			return err
		}

		zipPath := base + "/" + filepath.ToSlash(relPath)

		file, err := os.Open(pathname)
		if err != nil {
			return err
		}
		defer file.Close()

		w, err := writer.Create(zipPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(w, file)
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
