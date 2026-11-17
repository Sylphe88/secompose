// Package compose handles parsing and rewriting of docker compose arguments
// and compose file content.
package compose

import (
	"fmt"
	"os"
	"strings"
)

// defaultCandidates mirrors docker compose's own implicit file lookup order.
var defaultCandidates = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

// ParseResult is the outcome of scanning the CLI argument list.
type ParseResult struct {
	// ComposeFile is the path to the compose file that will be loaded into stdin.
	ComposeFile string
	// RewrittenArgs replaces the first -f/-file occurrence with "-f -" so that
	// docker compose reads the file from stdin instead of the filesystem.
	RewrittenArgs []string
}

// ParseArgs scans args (os.Args[1:]) for -f / --file, resolves the compose
// file path, and returns a ParseResult with the rewritten arg list.
//
// Only the first -f is replaced with "-f -"; subsequent ones (multi-file
// merges) are left untouched for docker compose to handle directly.
// If no -f is present the implicit default is discovered from the CWD.
func ParseArgs(args []string) (*ParseResult, error) {
	rewritten := make([]string, 0, len(args)+2)
	result := &ParseResult{}

	for i := 0; i < len(args); i++ {
		flag, val, ok := fileFlag(args, i)
		if !ok {
			rewritten = append(rewritten, args[i])
			continue
		}
		// Consume the token(s) that formed this flag
		i += flag.consumed - 1
		if result.ComposeFile == "" {
			// First -f: replace with stdin marker
			result.ComposeFile = val
			rewritten = append(rewritten, "-f", "-")
		} else {
			// Subsequent -f: keep as-is
			rewritten = append(rewritten, flag.raw...)
		}
	}

	if result.ComposeFile == "" {
		path, err := findDefault()
		if err != nil {
			return nil, err
		}
		result.ComposeFile = path
		rewritten = append([]string{"-f", "-"}, rewritten...)
	}

	result.RewrittenArgs = rewritten
	return result, nil
}

// fileFlagResult describes a matched -f / --file token.
type fileFlagResult struct {
	consumed int      // number of args[] tokens this flag occupies (1 or 2)
	raw      []string // original token(s), for verbatim pass-through
}

// fileFlag checks whether args[i] is a -f / --file flag and extracts its
// value. Returns (result, value, true) on match, ("", "", false) otherwise.
func fileFlag(args []string, i int) (fileFlagResult, string, bool) {
	arg := args[i]

	// --file=value (single token)
	if v, ok := strings.CutPrefix(arg, "--file="); ok {
		return fileFlagResult{consumed: 1, raw: []string{arg}}, v, true
	}

	// -f value  or  --file value (two tokens)
	if arg == "-f" || arg == "--file" {
		if i+1 >= len(args) {
			return fileFlagResult{}, "", false // malformed; leave to docker compose
		}
		val := args[i+1]
		return fileFlagResult{consumed: 2, raw: []string{arg, val}}, val, true
	}

	return fileFlagResult{}, "", false
}

// findDefault searches the CWD for a compose file using docker compose's
// implicit candidate order.
func findDefault() (string, error) {
	for _, name := range defaultCandidates {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("no compose file found (looked for %v)", defaultCandidates)
}
