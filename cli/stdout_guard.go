// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
)

// guardStdout swaps os.Stdout for os.Stderr and returns the real stdout writer.
// MCP stdio transport reserves os.Stdout for JSON-RPC frames; any stray
// fmt.Print* call from CLI code paths reused by the MCP server would corrupt
// the stream. After this call, all package-level os.Stdout writes go to
// stderr; the returned *os.File is the real stdout that the MCP SDK should
// use for protocol messages.
func guardStdout() *os.File {
	realStdout := os.Stdout
	os.Stdout = os.Stderr
	return realStdout
}
