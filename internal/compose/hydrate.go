package compose

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/sylphe88/secompose/internal/backend"
)

// lookupRe matches ${lookup(backend, path, key)} and the optional version=N.
//
// Capture groups:
//
//	1 — backend name  (e.g. "vault")
//	2 — path          (e.g. "secret/db/prod")
//	3 — key           (e.g. "password")
//	4 — version value (e.g. "3"), present only when version=N is given
var lookupRe = regexp.MustCompile(
	`\$\{lookup\(\s*(\w+)\s*,\s*([^,)]+?)\s*,\s*([^,)]+?)\s*(?:,\s*version=([^)]*?)\s*)?\)\}`,
)

// lookupKey uniquely identifies a secret lookup so that identical expressions
// scattered through the file are resolved only once.
type lookupKey struct {
	backendName string
	path        string
	key         string
	version     int // 0 = latest
}

// lookupResult holds the resolved value or an error for a single unique lookup.
type lookupResult struct {
	value string
	err   error
}

// job is a unit of work dispatched to a worker goroutine.
type job struct {
	lk      lookupKey
	backend backend.Backend
}

const workerCount = 4

// Hydrate scans data for ${lookup(...)} expressions, resolves all unique
// lookups concurrently (up to 4 goroutines), and returns the file with every
// expression replaced by its resolved value.
//
// On the first lookup failure the context is cancelled, remaining in-flight
// requests are abandoned, and the error is returned immediately.
func Hydrate(ctx context.Context, data []byte) ([]byte, error) {
	text := string(data)

	// --- 1. Collect all unique lookup keys from the file ---
	matches := lookupRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return data, nil // nothing to do
	}

	// Parse and deduplicate. Preserve insertion order for deterministic errors.
	seen := make(map[lookupKey]struct{})
	var unique []lookupKey

	for _, m := range matches {
		backendName := strings.TrimSpace(m[1])
		path := strings.TrimSpace(m[2])
		key := strings.TrimSpace(m[3])
		version, err := parseVersion(m[4]) // m[4] is "" when group absent
		if err != nil {
			return nil, fmt.Errorf("hydrate: %w", err)
		}

		lk := lookupKey{backendName, path, key, version}
		if _, exists := seen[lk]; !exists {
			seen[lk] = struct{}{}
			unique = append(unique, lk)
		}
	}

	// --- 2. Initialise backends (one per unique backend name) ---
	backends := make(map[string]backend.Backend)
	for _, lk := range unique {
		if _, ok := backends[lk.backendName]; ok {
			continue
		}
		b, err := backend.Get(lk.backendName)
		if err != nil {
			return nil, fmt.Errorf("hydrate: %w", err)
		}
		backends[lk.backendName] = b
	}

	// --- 3. Resolve all unique lookups via a bounded worker pool ---
	results, err := resolve(ctx, unique, backends)
	if err != nil {
		return nil, err
	}

	// --- 4. Substitute every occurrence in the file ---
	var subErr error
	hydrated := lookupRe.ReplaceAllStringFunc(text, func(match string) string {
		if subErr != nil {
			return match // already failed, don't bother
		}
		sub := lookupRe.FindStringSubmatch(match)
		version, _ := parseVersion(sub[4]) // already validated above
		lk := lookupKey{
			backendName: strings.TrimSpace(sub[1]),
			path:        strings.TrimSpace(sub[2]),
			key:         strings.TrimSpace(sub[3]),
			version:     version,
		}
		res := results[lk]
		if res.err != nil {
			subErr = res.err
			return match
		}
		return res.value
	})
	if subErr != nil {
		return nil, subErr
	}

	return []byte(hydrated), nil
}

// resolve dispatches jobs to workerCount goroutines and collects results.
// On the first error the context is cancelled and the error is returned.
func resolve(
	ctx context.Context,
	keys []lookupKey,
	backends map[string]backend.Backend,
) (map[lookupKey]lookupResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan job, len(keys))
	for _, lk := range keys {
		jobs <- job{lk: lk, backend: backends[lk.backendName]}
	}
	close(jobs)

	type indexedResult struct {
		lk  lookupKey
		res lookupResult
	}

	out := make(chan indexedResult, len(keys))

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				val, err := j.backend.Lookup(ctx, j.lk.path, j.lk.key, j.lk.version)
				out <- indexedResult{lk: j.lk, res: lookupResult{value: val, err: err}}
				if err != nil {
					cancel()
					return
				}
			}
		}()
	}

	wg.Wait()
	close(out)

	results := make(map[lookupKey]lookupResult, len(keys))
	for r := range out {
		results[r.lk] = r.res
		if r.res.err != nil {
			return nil, fmt.Errorf("hydrate: lookup(%s, %s, %s): %w",
				r.lk.backendName, r.lk.path, r.lk.key, r.res.err)
		}
	}
	return results, nil
}

// parseVersion converts the raw version string captured by the regex into an
// integer. An empty string returns 0 (latest). A non-integer value prints a
// warning to stderr and returns 0.
func parseVersion(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		_, _ = fmt.Fprintf(os.Stderr,
			"secret-compose: warning: version=%q is not a valid positive integer, defaulting to latest\n", raw)
		return 0, nil
	}
	return n, nil
}
