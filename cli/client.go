package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type client struct {
	token   string
	baseURL string
	http    *http.Client
}

type deployResult struct {
	liveURL string
	raw     string
}

type projectInfo struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	DeployCount int    `json:"deployCount"`
	LiveURL     string `json:"liveUrl"`
}

type userInfo struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func newClient(token string, baseURL string) *client {
	return &client{
		token:   token,
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{},
	}
}

func (c *client) deployDirectory(dirPath string, name string) (deployResult, error) {
	zipData, err := zipDirectory(dirPath)
	if err != nil {
		return deployResult{}, fmt.Errorf("create zip: %w", err)
	}

	return c.upload(name, "zip", filepath.Base(dirPath)+".zip", zipData)
}

func (c *client) deployZip(zipPath string, name string) (deployResult, error) {
	data, err := os.ReadFile(zipPath)
	if err != nil {
		return deployResult{}, err
	}

	return c.upload(name, "zip", filepath.Base(zipPath), data)
}

func (c *client) upload(name string, mode string, filename string, data []byte) (deployResult, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	writer.WriteField("name", name)
	writer.WriteField("mode", mode)

	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return deployResult{}, err
	}
	part.Write(data)
	writer.Close()

	req, err := http.NewRequest("POST", c.baseURL+"/api/uploads", body)
	if err != nil {
		return deployResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return deployResult{}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		var errResp struct{ Error string `json:"error"` }
		json.Unmarshal(respBody, &errResp)
		if errResp.Error != "" {
			return deployResult{}, fmt.Errorf("%s", errResp.Error)
		}
		return deployResult{}, fmt.Errorf("upload failed (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Project struct {
			LiveURL string `json:"liveUrl"`
		} `json:"project"`
	}
	json.Unmarshal(respBody, &result)

	return deployResult{
		liveURL: result.Project.LiveURL,
		raw:     string(respBody),
	}, nil
}

func (c *client) listProjects() ([]projectInfo, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/projects", nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized — check your VELORI_TOKEN")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Projects []projectInfo `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Projects, nil
}

func (c *client) getSession() (userInfo, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/session", nil)
	if err != nil {
		return userInfo{}, err
	}
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return userInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return userInfo{}, fmt.Errorf("unauthorized — check your VELORI_TOKEN")
	}

	var result struct {
		User userInfo `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return userInfo{}, err
	}

	return result.User, nil
}

func (c *client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
