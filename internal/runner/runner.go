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

func launchCompose(ctx context.Context, doc []byte, secrets compose.Secrets, args ...string) int {

	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Stdin = bytes.NewReader(doc)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	for name, s := range secrets {
		cmd.Env = append(cmd.Env, compose.ToEnvKeyValue(name, s))
	}

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

	data, err := os.ReadFile(parsed.ComposeFile)
	if err != nil {
		return 1
	}

	secrets, err := compose.ParseSecrets(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "secompose: reading %q: %v\n", parsed.ComposeFile, err)
		return 1
	}

	// secret_names := slices.Collect(maps.Keys(secrets))
	// fmt.Println("secompose: Found secrets:", secret_names)

	if err := secrets.ResolveAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "secompose: failed resolution: %v\n", err)
		return 1
	}

	// for name, s := range secrets {
	// 	fmt.Printf("Secret %q -> %q", name, s.Value)
	// }

	return launchCompose(ctx, data, secrets, parsed.RewrittenArgs...)

}
