// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// cmdMCP runs the MCP server over stdio. It reuses the existing config and
// HTTP client so behavior stays in lock-step with the CLI surface.
func cmdMCP(_ []string) {
	// Reserve real stdout for the JSON-RPC framer; everything else goes to
	// stderr so stray prints can't corrupt the protocol stream.
	realStdout := guardStdout()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "protopen", Version: version},
		&mcp.ServerOptions{
			Instructions: mcpInstructions,
		},
	)

	registerMCPTools(server)

	// Wrap both ends so the SDK can't close the real OS streams when the
	// session ends — those handles outlive any single MCP session.
	transport := &mcp.IOTransport{
		Reader: nopReadCloser{os.Stdin},
		Writer: nopWriteCloser{realStdout},
	}
	if err := server.Run(context.Background(), transport); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server: %v\n", err)
		os.Exit(1)
	}
}

// nopWriteCloser turns any io.Writer into an io.WriteCloser whose Close is a
// no-op. Used to keep the SDK from closing os.Stdout at session end.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// nopReadCloser is the read-side counterpart, used to protect os.Stdin.
type nopReadCloser struct{ io.Reader }

func (nopReadCloser) Close() error { return nil }

const mcpInstructions = `Protopen MCP server.

Before using any tool, the user must have authenticated by running:
  protopen login

Tools share the CLI configuration at ~/.protopen/config.json. Pass an "org"
argument to scope a tool to a specific organization; omit it to use the
user's default org. The deploy tool requires an absolute path because the
MCP server has no notion of the agent's working directory.`

// mcpClient builds a fresh *client from the current config/env. It is called
// per tool invocation so that token rotation via the CLI takes effect without
// restarting the MCP server.
func mcpClient() (*client, error) {
	token := resolveToken("")
	if token == "" {
		return nil, &AuthError{Msg: "no API token configured — run `protopen login` in a terminal"}
	}
	return newClient(token, resolveURL("")), nil
}

// mcpSiteContext bundles the per-tool prelude shared by every site-scoped
// handler: resolve the API client, resolve the org (explicit arg wins over
// config), and look up the site by name/slug. Returns errors already wrapped
// by mcpError so handlers can return them directly.
func mcpSiteContext(siteArg, orgArg string) (*client, siteInfo, error) {
	c, err := mcpClient()
	if err != nil {
		return nil, siteInfo{}, mcpError(err)
	}
	site, err := c.findSite(siteArg, resolveOrg(orgArg))
	if err != nil {
		return nil, siteInfo{}, mcpError(err)
	}
	return c, site, nil
}

// mcpError prefixes the message with the typed category so agents can branch
// on AUTH / NOT_FOUND / VALIDATION / FORBIDDEN / SERVER / NETWORK without
// parsing human prose. The error chain is preserved via %w so callers that
// want to use errors.As against the wrapped result still can.
func mcpError(err error) error {
	if err == nil {
		return nil
	}
	var (
		authErr   *AuthError
		notFound  *NotFoundError
		valErr    *ValidationError
		forbidden *ForbiddenError
		serverErr *ServerError
		netErr    *NetworkError
	)
	switch {
	case errors.As(err, &authErr):
		return fmt.Errorf("AUTH: %w", err)
	case errors.As(err, &notFound):
		return fmt.Errorf("NOT_FOUND: %w", err)
	case errors.As(err, &valErr):
		return fmt.Errorf("VALIDATION: %w", err)
	case errors.As(err, &forbidden):
		return fmt.Errorf("FORBIDDEN: %w", err)
	case errors.As(err, &serverErr):
		return fmt.Errorf("SERVER: %w", err)
	case errors.As(err, &netErr):
		return fmt.Errorf("NETWORK: %w", err)
	}
	return err
}

// requireAbsolutePath enforces that the path supplied to the MCP deploy tool
// is absolute. Stdio MCP has no shared cwd with the agent, so relative paths
// cannot be resolved reliably.
func requireAbsolutePath(p string) error {
	if !filepath.IsAbs(p) {
		return &ValidationError{Msg: fmt.Sprintf("path must be absolute, got %q — pass the full path from /", p)}
	}
	if _, err := os.Stat(p); err != nil {
		return &NotFoundError{Msg: fmt.Sprintf("path not accessible: %v", err), Err: err}
	}
	return nil
}
