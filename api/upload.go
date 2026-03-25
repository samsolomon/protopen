package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func (app *application) uploadsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	prepared, err := app.prepareUpload(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := validateUpload(prepared.request); err != nil {
		cleanupPreparedUpload(prepared)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	user, err := app.requireSessionUser(r)
	if err != nil {
		cleanupPreparedUpload(prepared)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if app.store != nil && prepared.normalizedTo != "" {
		r2Prefix, uploadErr := app.uploadToR2(r.Context(), prepared)
		if uploadErr != nil {
			cleanupPreparedUpload(prepared)
			log.Printf("r2 upload: %v", uploadErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not store uploaded files"})
			return
		}
		prepared.normalizedTo = r2Prefix
	}

	result, err := app.upsertProjectFromUpload(r.Context(), user.Email, prepared)
	if err != nil {
		cleanupPreparedUpload(prepared)
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "multiple existing projects") {
			status = http.StatusConflict
		}

		log.Printf("upsert upload: %v", err)
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	if app.store != nil {
		cleanupPreparedUpload(prepared)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "Upload staged and recorded.",
		"status":  "queued",
		"project": result,
	})
}

func (app *application) prepareUpload(w http.ResponseWriter, r *http.Request) (preparedUpload, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return app.prepareMultipartUpload(w, r)
	}

	var payload uploadRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return preparedUpload{}, fmt.Errorf("invalid upload payload")
	}

	return preparedUpload{request: payload}, nil
}

func (app *application) prepareMultipartUpload(w http.ResponseWriter, r *http.Request) (prepared preparedUpload, err error) {
	r.Body = http.MaxBytesReader(w, r.Body, 110*1024*1024)
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		return preparedUpload{}, fmt.Errorf("invalid upload payload")
	}

	payload := uploadRequest{
		Name: strings.TrimSpace(r.FormValue("name")),
		Mode: strings.TrimSpace(r.FormValue("mode")),
	}

	paths := []string{}
	if rawPaths := strings.TrimSpace(r.FormValue("paths")); rawPaths != "" {
		if err := json.Unmarshal([]byte(rawPaths), &paths); err != nil {
			return preparedUpload{}, fmt.Errorf("invalid upload path manifest")
		}
	}

	headers := fileHeadersFromForm(r.MultipartForm)
	if len(paths) > 0 && len(paths) != len(headers) {
		return preparedUpload{}, fmt.Errorf("file path manifest does not match upload count")
	}

	ingestID := generateID("ingest")
	uploadRoot := filepath.Join(app.ingestRoot, ingestID)
	rawRoot := filepath.Join(uploadRoot, "raw")
	normalizedRoot := filepath.Join(uploadRoot, "normalized")
	defer func() {
		if err != nil && uploadRoot != "" {
			_ = os.RemoveAll(uploadRoot)
		}
	}()

	if err := os.MkdirAll(rawRoot, 0o755); err != nil {
		return preparedUpload{}, err
	}
	if err := os.MkdirAll(normalizedRoot, 0o755); err != nil {
		return preparedUpload{}, err
	}

	payload.Files = make([]fileMeta, 0, len(headers))
	for index, header := range headers {
		path := header.Filename
		if len(paths) > 0 {
			path = paths[index]
		}

		normalizedPath, err := normalizeUploadPath(path)
		if err != nil {
			return preparedUpload{}, err
		}

		rawFilePath := filepath.Join(rawRoot, fmt.Sprintf("%04d-%s", index, filepath.Base(normalizedPath)))
		if err := writeMultipartFile(header, rawFilePath); err != nil {
			return preparedUpload{}, err
		}

		if payload.Mode == "zip" {
			if err := extractZipToDir(rawFilePath, normalizedRoot); err != nil {
				return preparedUpload{}, err
			}
		} else {
			destinationPath := filepath.Join(normalizedRoot, filepath.FromSlash(normalizedPath))
			if err := copyFile(rawFilePath, destinationPath); err != nil {
				return preparedUpload{}, err
			}
		}

		payload.Files = append(payload.Files, fileMeta{
			Name: header.Filename,
			Size: header.Size,
			Path: normalizedPath,
		})
	}

	if payload.Mode == "zip" {
		if _, err := collectFiles(normalizedRoot); err != nil {
			return preparedUpload{}, err
		}
	}

	siteRoot, err := detectSiteRoot(normalizedRoot)
	if err != nil {
		return preparedUpload{}, err
	}

	normalizedFiles, err := collectFiles(siteRoot)
	if err != nil {
		return preparedUpload{}, err
	}
	payload.Files = normalizedFiles

	prepared = preparedUpload{request: payload, ingestRoot: uploadRoot, normalizedTo: siteRoot}
	return prepared, nil
}

func cleanupPreparedUpload(prepared preparedUpload) {
	if prepared.ingestRoot == "" {
		return
	}
	_ = os.RemoveAll(prepared.ingestRoot)
}

func writeMultipartFile(header *multipart.FileHeader, destination string) error {
	file, err := header.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer output.Close()

	_, err = io.Copy(output, file)
	return err
}

func copyFile(source string, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer output.Close()

	_, err = io.Copy(output, input)
	return err
}

func normalizeUploadPath(value string) (string, error) {
	cleaned := filepath.ToSlash(strings.TrimSpace(value))
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = path.Clean(cleaned)
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("invalid file path")
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("unsafe file path: %s", value)
	}
	return cleaned, nil
}

func extractZipToDir(zipPath string, destinationRoot string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("invalid zip upload")
	}
	defer reader.Close()

	const maxArchiveFiles = 5000
	const maxArchiveBytes = int64(100 * 1024 * 1024)

	var totalBytes int64
	fileCount := 0
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip uploads cannot contain symlinks")
		}

		normalizedPath, err := normalizeUploadPath(file.Name)
		if err != nil {
			return err
		}

		fileCount++
		if fileCount > maxArchiveFiles {
			return fmt.Errorf("zip upload contains too many files")
		}

		totalBytes += int64(file.UncompressedSize64)
		if totalBytes > maxArchiveBytes {
			return fmt.Errorf("zip upload exceeds the 100MB extracted size limit")
		}

		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(normalizedPath))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return err
		}

		input, err := file.Open()
		if err != nil {
			return err
		}

		output, err := os.Create(destinationPath)
		if err != nil {
			input.Close()
			return err
		}

		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		inputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if inputErr != nil {
			return inputErr
		}
	}

	return nil
}

func collectFiles(root string) ([]fileMeta, error) {
	files := make([]fileMeta, 0)
	err := filepath.WalkDir(root, func(pathname string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(root, pathname)
		if err != nil {
			return err
		}

		normalizedPath, err := normalizeUploadPath(relativePath)
		if err != nil {
			return err
		}

		files = append(files, fileMeta{
			Name: filepath.Base(normalizedPath),
			Size: info.Size(),
			Path: normalizedPath,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i int, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func detectSiteRoot(root string) (string, error) {
	var candidates []string
	err := filepath.WalkDir(root, func(pathname string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(entry.Name(), "index.html") {
			return nil
		}
		candidates = append(candidates, filepath.Dir(pathname))
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no index.html found - Velori hosts built static sites only")
	}

	sort.Slice(candidates, func(i int, j int) bool {
		depthI := strings.Count(filepath.ToSlash(candidates[i]), "/")
		depthJ := strings.Count(filepath.ToSlash(candidates[j]), "/")
		if depthI == depthJ {
			return candidates[i] < candidates[j]
		}
		return depthI < depthJ
	})

	return candidates[0], nil
}

func fileHeadersFromForm(form *multipart.Form) []*multipart.FileHeader {
	if form == nil || len(form.File) == 0 {
		return nil
	}

	keys := make([]string, 0, len(form.File))
	for key := range form.File {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	headers := make([]*multipart.FileHeader, 0)
	for _, key := range keys {
		headers = append(headers, form.File[key]...)
	}

	return headers
}

func validateUpload(payload uploadRequest) error {
	cleanName := strings.TrimSpace(payload.Name)
	if cleanName == "" {
		return fmt.Errorf("upload name is required")
	}

	if len(payload.Files) == 0 {
		return fmt.Errorf("select a folder or zip to deploy")
	}

	const maxDeploySize = 100 * 1024 * 1024
	const maxFileSize = 20 * 1024 * 1024

	totalSize := int64(0)
	hasIndexHTML := false
	hasZip := false

	for _, file := range payload.Files {
		totalSize += file.Size

		if file.Size > maxFileSize {
			return fmt.Errorf("%s exceeds the 20MB per-file limit", file.Name)
		}

		path := strings.ToLower(strings.TrimSpace(file.Path))
		name := strings.ToLower(strings.TrimSpace(file.Name))
		if strings.HasSuffix(name, ".zip") {
			hasZip = true
		}

		if name == "index.html" || strings.HasSuffix(path, "/index.html") {
			hasIndexHTML = true
		}
	}

	if totalSize > maxDeploySize {
		return fmt.Errorf("deploy exceeds the 100MB upload limit")
	}

	if payload.Mode == "zip" {
		if hasZip {
			if len(payload.Files) != 1 {
				return fmt.Errorf("zip uploads must contain exactly one .zip file")
			}
			return nil
		}

		if !hasIndexHTML {
			return fmt.Errorf("no index.html found in extracted zip - Velori hosts built static sites only")
		}
		return nil
	}

	if !hasIndexHTML {
		return fmt.Errorf("no index.html found - Velori hosts built static sites only")
	}

	return nil
}

func (app *application) uploadToR2(ctx context.Context, prepared preparedUpload) (string, error) {
	prefix := generateID("deploy")
	siteRoot := prepared.normalizedTo

	err := filepath.WalkDir(siteRoot, func(pathname string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, relErr := filepath.Rel(siteRoot, pathname)
		if relErr != nil {
			return relErr
		}

		key := prefix + "/" + filepath.ToSlash(relativePath)

		file, openErr := os.Open(pathname)
		if openErr != nil {
			return openErr
		}
		defer file.Close()

		contentType := mime.TypeByExtension(filepath.Ext(pathname))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		return app.store.upload(ctx, key, file, contentType)
	})
	if err != nil {
		return "", err
	}

	return prefix, nil
}
