// Package backend defines the interface for secret storage backends and
// provides a registry of available implementations.
package backend

import "context"

// Backend is the interface that all secret storage backends must implement.
// version=0 means "latest" — backends should retrieve the most recent version
// of the secret when version is 0.
type Backend interface {
	Lookup(ctx context.Context, path, key string, version int) (string, error)
}
