# Protopen

## API docs

When modifying, adding, or removing API endpoints in `api/*.go`, update `docs/api-reference.md` to reflect the change. This file is the authoritative API reference.

Maintain the existing structure: sections grouped by feature, with request/response examples, status codes, and the constraints table at the bottom.

## CLI / MCP parity

The CLI (`cli/main.go`) and the MCP server (`cli/mcp_tools.go`, `cli/mcp_handlers.go`) are two front doors to the same HTTP API. When a CLI command is added, removed, or its behavior changes, update the matching MCP tool in the same change so the surfaces don't drift. Both share the HTTP client and config loader in `cli/client.go` and `cli/config.go`.
