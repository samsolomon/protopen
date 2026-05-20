// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input/output structs are the schema source of truth — the SDK derives a
// JSON Schema from these struct tags. Keep field names lowerCamelCase to
// match the JSON conventions used elsewhere in the API.

type deployInput struct {
	Path    string `json:"path" jsonschema:"absolute filesystem path to the directory or .zip to deploy"`
	Name    string `json:"name,omitempty" jsonschema:"optional site name (defaults to the directory or zip basename)"`
	Label   string `json:"label,omitempty" jsonschema:"optional human-readable deploy label"`
	Org     string `json:"org,omitempty" jsonschema:"optional organization slug; omit for the user's default org"`
	Public  *bool  `json:"public,omitempty" jsonschema:"set true to force public, false to force private; omit to inherit the instance default"`
}

type deployOutput struct {
	SiteID  string `json:"siteId"`
	LiveURL string `json:"liveUrl"`
}

type listSitesInput struct {
	Org string `json:"org,omitempty" jsonschema:"optional organization slug; omit for the user's default org"`
}

type listSitesOutput struct {
	Sites []siteInfo `json:"sites"`
}

type listDeploysInput struct {
	Site string `json:"site" jsonschema:"site name or slug"`
	Org  string `json:"org,omitempty" jsonschema:"optional organization slug; omit for the user's default org"`
}

type listDeploysOutput struct {
	Deploys []deployInfo `json:"deploys"`
}

type rollbackInput struct {
	Site     string `json:"site" jsonschema:"site name or slug"`
	DeployID string `json:"deployId" jsonschema:"id of the deploy to roll back to (from list_deploys)"`
	Org      string `json:"org,omitempty" jsonschema:"optional organization slug; omit for the user's default org"`
}

type rollbackOutput struct {
	OK bool `json:"ok"`
}

type listCommentsInput struct {
	Site   string `json:"site" jsonschema:"site name or slug"`
	Status string `json:"status,omitempty" jsonschema:"open | resolved | all (defaults to open)"`
	Deploy string `json:"deployId,omitempty" jsonschema:"optional deploy id to scope comments to"`
	Org    string `json:"org,omitempty" jsonschema:"optional organization slug; omit for the user's default org"`
}

type listCommentsOutput struct {
	Comments []commentInfo `json:"comments"`
}

// boolPtr returns a pointer to v, used for the SDK's optional *bool annotation
// fields (DestructiveHint, IdempotentHint, ReadOnlyHint).
func boolPtr(v bool) *bool { return &v }

// registerMCPTools wires the five v1 tools onto the server. Tool descriptions
// are the agent-facing contract; keep them precise and action-oriented.
func registerMCPTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "deploy",
		Description: "Upload a directory or .zip to Protopen and make it the live deploy for the named site. Creates the site if it does not exist. The path must be absolute.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Deploy site",
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
		},
	}, deployHandler)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_sites",
		Description: "List the sites visible to the current user, optionally scoped to an organization.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "List sites",
			ReadOnlyHint: true,
		},
	}, listSitesHandler)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_deploys",
		Description: "List the deploy history for a site, newest first. Use the returned ids with the rollback tool.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "List deploys",
			ReadOnlyHint: true,
		},
	}, listDeploysHandler)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "rollback",
		Description: "Roll a site's live deploy back to a previous deploy id. Use list_deploys to find the id.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Roll back deploy",
			DestructiveHint: boolPtr(true),
			IdempotentHint:  true,
		},
	}, rollbackHandler)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_comments",
		Description: "List reviewer comments on a site. Defaults to open comments. Set status to \"resolved\" or \"all\" to widen the filter.",
		Annotations: &mcp.ToolAnnotations{
			Title:        "List comments",
			ReadOnlyHint: true,
		},
	}, listCommentsHandler)
}

func deployHandler(_ context.Context, _ *mcp.CallToolRequest, in deployInput) (*mcp.CallToolResult, deployOutput, error) {
	if err := requireAbsolutePath(in.Path); err != nil {
		return nil, deployOutput{}, mcpError(err)
	}
	c, err := mcpClient()
	if err != nil {
		return nil, deployOutput{}, mcpError(err)
	}

	info, err := os.Stat(in.Path)
	if err != nil {
		return nil, deployOutput{}, mcpError(&NotFoundError{Msg: err.Error(), Err: err})
	}
	name := in.Name
	if name == "" {
		name = info.Name()
		if !info.IsDir() {
			name = stripZipExt(name)
		}
	}
	org := resolveOrg(in.Org)
	git := detectGitMeta(in.Path)

	var result deployResult
	if info.IsDir() {
		result, err = c.deployDirectory(in.Path, name, in.Label, org, git, in.Public)
	} else {
		result, err = c.deployZip(in.Path, name, in.Label, org, git, in.Public)
	}
	if err != nil {
		return nil, deployOutput{}, mcpError(err)
	}
	return nil, deployOutput{SiteID: result.siteID, LiveURL: result.liveURL}, nil
}
