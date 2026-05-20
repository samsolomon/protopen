// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listSitesHandler(_ context.Context, _ *mcp.CallToolRequest, in listSitesInput) (*mcp.CallToolResult, listSitesOutput, error) {
	c, err := mcpClient()
	if err != nil {
		return nil, listSitesOutput{}, mcpError(err)
	}
	sites, err := c.listSites(resolveOrg(in.Org))
	if err != nil {
		return nil, listSitesOutput{}, mcpError(err)
	}
	return nil, listSitesOutput{Sites: sites}, nil
}

func listDeploysHandler(_ context.Context, _ *mcp.CallToolRequest, in listDeploysInput) (*mcp.CallToolResult, listDeploysOutput, error) {
	c, site, err := mcpSiteContext(in.Site, in.Org)
	if err != nil {
		return nil, listDeploysOutput{}, err
	}
	deploys, err := c.listDeploys(site.ID)
	if err != nil {
		return nil, listDeploysOutput{}, mcpError(err)
	}
	return nil, listDeploysOutput{Deploys: deploys}, nil
}

func rollbackHandler(_ context.Context, _ *mcp.CallToolRequest, in rollbackInput) (*mcp.CallToolResult, rollbackOutput, error) {
	c, site, err := mcpSiteContext(in.Site, in.Org)
	if err != nil {
		return nil, rollbackOutput{}, err
	}
	if err := c.rollback(site.ID, in.DeployID); err != nil {
		return nil, rollbackOutput{}, mcpError(err)
	}
	return nil, rollbackOutput{OK: true}, nil
}

func listCommentsHandler(_ context.Context, _ *mcp.CallToolRequest, in listCommentsInput) (*mcp.CallToolResult, listCommentsOutput, error) {
	c, site, err := mcpSiteContext(in.Site, in.Org)
	if err != nil {
		return nil, listCommentsOutput{}, err
	}
	status := in.Status
	if status == "" {
		status = "open"
	}
	comments, err := c.listComments(site.ID, listCommentsOpts{DeployID: in.Deploy, Status: status})
	if err != nil {
		return nil, listCommentsOutput{}, mcpError(err)
	}
	return nil, listCommentsOutput{Comments: comments}, nil
}
