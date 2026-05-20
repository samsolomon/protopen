// SPDX-License-Identifier: AGPL-3.0-only

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
	siteID  string
	liveURL string
	raw     string
}

type siteInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	DeployCount int    `json:"deployCount"`
	LiveURL     string `json:"liveUrl"`
	IsPublic    bool   `json:"isPublic"`
}

type deployInfo struct {
	ID               string  `json:"id"`
	Status           string  `json:"status"`
	Label            *string `json:"label"`
	FileCount        int     `json:"fileCount"`
	CreatedAt        string  `json:"createdAt"`
	IsCurrent        bool    `json:"isCurrent"`
	GitCommitHash    *string `json:"gitCommitHash"`
	GitBranch        *string `json:"gitBranch"`
	GitCommitMessage *string `json:"gitCommitMessage"`
	GitDirty         *bool   `json:"gitDirty"`
	GitAuthor        *string `json:"gitAuthor"`
	GitRemoteURL     *string `json:"gitRemoteURL"`
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

func (c *client) deployDirectory(dirPath string, name string, label string, org string, git *gitMeta, isPublic *bool) (deployResult, error) {
	zipData, err := zipDirectory(dirPath)
	if err != nil {
		return deployResult{}, fmt.Errorf("create zip: %w", err)
	}

	return c.upload(name, "zip", filepath.Base(dirPath)+".zip", zipData, label, org, git, isPublic)
}

func (c *client) deployZip(zipPath string, name string, label string, org string, git *gitMeta, isPublic *bool) (deployResult, error) {
	data, err := os.ReadFile(zipPath)
	if err != nil {
		return deployResult{}, err
	}

	return c.upload(name, "zip", filepath.Base(zipPath), data, label, org, git, isPublic)
}

func (c *client) upload(name string, mode string, filename string, data []byte, label string, org string, git *gitMeta, isPublic *bool) (deployResult, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	writer.WriteField("name", name)
	writer.WriteField("mode", mode)
	if label != "" {
		writer.WriteField("label", label)
	}
	// Absent is_public field = inherit the instance default_site_private
	// setting. Only set when the caller passed --public/--private.
	if isPublic != nil {
		if *isPublic {
			writer.WriteField("is_public", "true")
		} else {
			writer.WriteField("is_public", "false")
		}
	}
	if git != nil {
		writer.WriteField("git_commit_hash", git.CommitHash)
		writer.WriteField("git_branch", git.Branch)
		writer.WriteField("git_commit_message", git.CommitMessage)
		if git.Dirty {
			writer.WriteField("git_dirty", "true")
		} else {
			writer.WriteField("git_dirty", "false")
		}
		writer.WriteField("git_author", git.Author)
		writer.WriteField("git_remote_url", git.RemoteURL)
	}

	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return deployResult{}, err
	}
	part.Write(data)
	writer.Close()

	endpoint := c.baseURL + "/api/uploads"
	if org != "" {
		endpoint += "?org=" + org
	}
	req, err := http.NewRequest("POST", endpoint, body)
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
		Site struct {
			ID      string `json:"id"`
			LiveURL string `json:"liveUrl"`
		} `json:"site"`
	}
	json.Unmarshal(respBody, &result)

	return deployResult{
		siteID:  result.Site.ID,
		liveURL: result.Site.LiveURL,
		raw:     string(respBody),
	}, nil
}

func (c *client) listSites(org string) ([]siteInfo, error) {
	endpoint := c.baseURL + "/api/sites"
	if org != "" {
		endpoint += "?org=" + org
	}
	req, err := http.NewRequest("GET", endpoint, nil)
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
		return nil, fmt.Errorf("unauthorized — check your PROTOPEN_TOKEN")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Sites []siteInfo `json:"sites"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Sites, nil
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
		return userInfo{}, fmt.Errorf("unauthorized — check your PROTOPEN_TOKEN")
	}

	var result struct {
		User userInfo `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return userInfo{}, err
	}

	return result.User, nil
}

func (c *client) findSite(nameOrSlug string, org string) (siteInfo, error) {
	sites, err := c.listSites(org)
	if err != nil {
		return siteInfo{}, err
	}

	nameOrSlug = strings.ToLower(strings.TrimSpace(nameOrSlug))
	for _, s := range sites {
		if strings.ToLower(s.Name) == nameOrSlug || strings.ToLower(s.Slug) == nameOrSlug {
			return s, nil
		}
	}

	return siteInfo{}, fmt.Errorf("site %q not found", nameOrSlug)
}

func (c *client) listDeploys(siteID string) ([]deployInfo, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/api/sites/"+siteID+"/deploys", nil)
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
		return nil, fmt.Errorf("unauthorized — check your PROTOPEN_TOKEN")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Deploys []deployInfo `json:"deploys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Deploys, nil
}

type commentAuthor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type commentInfo struct {
	ID         string          `json:"id"`
	SiteID     string          `json:"siteId"`
	DeployID   string          `json:"deployId"`
	PagePath   string          `json:"pagePath"`
	PinX       *float64        `json:"pinX"`
	PinY       *float64        `json:"pinY"`
	Body       string          `json:"body"`
	ParentID   *string         `json:"parentId"`
	ResolvedAt *string         `json:"resolvedAt"`
	CreatedAt  string          `json:"createdAt"`
	Author     *commentAuthor  `json:"author"`
}

func (c *client) listComments(siteID string, opts struct{ DeployID, Status string }) ([]commentInfo, error) {
	url := c.baseURL + "/api/sites/" + siteID + "/comments"
	params := []string{}
	if opts.DeployID != "" {
		params = append(params, "deployId="+opts.DeployID)
	}
	if opts.Status != "" {
		params = append(params, "status="+opts.Status)
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	req, err := http.NewRequest("GET", url, nil)
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
		return nil, fmt.Errorf("unauthorized — check your PROTOPEN_TOKEN")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("forbidden — your token is not a member of this site's org")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed (HTTP %d)", resp.StatusCode)
	}

	var result struct {
		Comments []commentInfo `json:"comments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Comments, nil
}

func (c *client) rollback(siteID string, deployID string) error {
	body, _ := json.Marshal(map[string]string{"deployId": deployID})
	req, err := http.NewRequest("POST", c.baseURL+"/api/sites/"+siteID+"/rollback", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized — check your PROTOPEN_TOKEN")
	}
	if resp.StatusCode != http.StatusOK {
		var errResp struct{ Error string `json:"error"` }
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("%s", errResp.Error)
		}
		return fmt.Errorf("rollback failed (HTTP %d)", resp.StatusCode)
	}

	return nil
}

func (c *client) updateVisibility(siteID string, isPublic bool) error {
	body, _ := json.Marshal(map[string]bool{"isPublic": isPublic})
	req, err := http.NewRequest("PATCH", c.baseURL+"/api/sites/"+siteID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized — check your PROTOPEN_TOKEN")
	}
	if resp.StatusCode != http.StatusOK {
		var errResp struct{ Error string `json:"error"` }
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("%s", errResp.Error)
		}
		return fmt.Errorf("update failed (HTTP %d)", resp.StatusCode)
	}

	return nil
}

func (c *client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
