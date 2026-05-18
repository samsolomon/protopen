// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// loadDotenv reads `.env` from the working directory or the nearest parent and
// applies any KEY=value pairs that aren't already set in the process
// environment. It mirrors the small subset of dotenv that `set -a; source .env`
// would: # comments, blank lines, optional `export ` prefix, and "..."/'...'
// quoted values. Variable substitution is intentionally not supported — the
// goal is to make `go run .` work without shell sourcing, not to replace
// envsubst.
//
// Already-set environment variables win over file values so a shell-exported
// override always takes precedence. Errors other than file-not-found are
// returned so misconfigured files surface loudly instead of silently
// half-loading.
func loadDotenv() error {
	path, ok := findDotenv()
	if !ok {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	pairs, err := parseDotenv(file)
	if err != nil {
		return err
	}
	for k, v := range pairs {
		if _, present := os.LookupEnv(k); present {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

// findDotenv walks up from the working directory looking for a `.env`. Walking
// up keeps things sane when the binary is invoked from `api/` (the README's
// recommended invocation) while the file lives at the repo root.
func findDotenv() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(dir, ".env")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// parseDotenv reads a small subset of the dotenv format. Unparseable lines
// return an error rather than being silently skipped so config typos are
// caught at startup.
func parseDotenv(r interface {
	Read(p []byte) (int, error)
}) (map[string]string, error) {
	out := make(map[string]string)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return nil, &dotenvError{line: lineNum, msg: "missing `=` in `" + line + "`"}
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		if !isValidEnvKey(key) {
			return nil, &dotenvError{line: lineNum, msg: "invalid key `" + key + "`"}
		}

		if value != "" {
			if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
				(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
				if len(value) < 2 {
					return nil, &dotenvError{line: lineNum, msg: "unterminated quote"}
				}
				value = value[1 : len(value)-1]
			} else if i := strings.IndexByte(value, '#'); i >= 0 {
				// Inline comments are only stripped for unquoted values.
				value = strings.TrimSpace(value[:i])
			}
		}
		out[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func isValidEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

type dotenvError struct {
	line int
	msg  string
}

func (e *dotenvError) Error() string {
	return ".env line " + strconv.Itoa(e.line) + ": " + e.msg
}
