package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Vault is a Hashicorp Vault KV v2 backend.
//
// Authentication is via a static token read from the environment at
// construction time (VAULT_ADDR + VAULT_TOKEN).
//
// Path convention: the first segment of path is the KV mount, the remainder
// is the secret path within that mount.
//
//	secret/db/prod  →  mount=secret  subpath=db/prod
//	secret/db       →  mount=secret  subpath=db
type Vault struct {
	addr   string
	token  string
	client *http.Client
}

// NewVault constructs a Vault backend by reading VAULT_ADDR and VAULT_TOKEN
// from the environment. Returns an error if either variable is unset or empty.
func NewVault() (*Vault, error) {
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		return nil, fmt.Errorf("VAULT_ADDR is not set")
	}
	token := os.Getenv("VAULT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("VAULT_TOKEN is not set")
	}
	return &Vault{
		addr:   strings.TrimRight(addr, "/"),
		token:  token,
		client: &http.Client{},
	}, nil
}

// Lookup retrieves the value of key from the KV v2 secret at path.
// version=0 fetches the latest version.
//
// URL pattern: GET {addr}/v1/{mount}/data/{subpath}[?version={version}]
// Response path: .data.data.{key}
func (v *Vault) Lookup(ctx context.Context, path, key string, version int) (string, error) {
	mount, subpath, err := splitPath(path)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/v1/%s/data/%s", v.addr, mount, subpath)
	if version > 0 {
		url = fmt.Sprintf("%s?version=%d", url, version)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("vault: building request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.token)

	resp, err := v.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("vault: reading response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// handled below
	case http.StatusNotFound:
		return "", fmt.Errorf("vault: secret not found at %q", path)
	case http.StatusForbidden:
		return "", fmt.Errorf("vault: permission denied for %q", path)
	default:
		return "", fmt.Errorf("vault: unexpected status %d for %q: %s", resp.StatusCode, path, body)
	}

	// Parse: {"data":{"data":{"key":"value"}}}
	var envelope struct {
		Data struct {
			Data map[string]interface{} `json:"data"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("vault: decoding response for %q: %w", path, err)
	}

	val, ok := envelope.Data.Data[key]
	if !ok {
		return "", fmt.Errorf("vault: key %q not found in secret at %q", key, path)
	}

	// Vault stores all values as strings in KV v2, but the JSON type is
	// interface{} — coerce to string defensively.
	switch s := val.(type) {
	case string:
		return s, nil
	default:
		return fmt.Sprintf("%v", val), nil
	}
}

// splitPath splits "mount/sub/path" into ("mount", "sub/path").
// A single-segment path (no slash) is an error — Vault KV v2 always requires
// a mount plus at least one path segment.
func splitPath(path string) (mount, subpath string, err error) {
	path = strings.TrimSpace(path)
	idx := strings.Index(path, "/")
	if idx < 0 || idx == len(path)-1 {
		return "", "", fmt.Errorf("vault: path %q must have at least two segments (mount/subpath)", path)
	}
	return path[:idx], path[idx+1:], nil
}
