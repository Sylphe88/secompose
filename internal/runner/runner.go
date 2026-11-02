package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/sylphe88/secompose/internal/compose"
)

// Run executes `docker compose` with the provided arguments.
//
// Pipeline:
//  1. Parse args to locate the compose file and rewrite "-f <file>" → "-f -"
//  2. Read the compose file into memory
//  3. Look for secrets: > x-secompose-* blocks and create dynamic provider-based lookups
//  4. Hydrate: resolve lookups and read secrets from provider
//  5. Rewrite compose file to point the secret to a predefined env var
//  6. Pipe the hydrated YAML to docker compose via stdin, pass secrets as env vars to the process
//
// stdout and stderr are inherited from the caller so interactive commands and
// TTY output work correctly. The exit code of docker compose is returned.
func Run(args []string) int {
	ctx := context.Background()

	parsed, err := compose.ParseArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "secompose: %v\n", err)
		return 1
	}

	secrets, err := compose.parseSecrets(parsed.ComposeFile)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "secompose: reading %q: %v\n", parsed.ComposeFile, err)
		return 1
	}

	hydrated, err := compose.Hydrate(ctx, secrets)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "secret-compose: %v\n", err)
		return 1
	}

	cmdArgs := append([]string{"compose"}, parsed.RewrittenArgs...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Stdin = bytes.NewReader(hydrated)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		_, _ = fmt.Fprintf(os.Stderr, "secret-compose: %v\n", err)
		return 1
	}

	return 0
}
