package backend

import (
	"context"
	"fmt"
	"strconv"

	vault "github.com/hashicorp/vault/api"
)

type Vault struct {
	Ref   `yaml:",inline"`
	Mount string `yaml:"namespace"`
}

func (v *Vault) Load(ctx context.Context) (string, error) {
	c, err := vault.NewClient(vault.DefaultConfig())
	if err != nil {
		return "", err
	}

	kv := c.KVv2(v.Mount)
	ver, err := strconv.Atoi(v.Version)
	if err != nil {
		fmt.Println("Version shall be a valid integer or 'latest'")
		return "", err
	}
	var s *vault.KVSecret
	if v.Version != "latest" {
		s, err = kv.GetVersion(ctx, v.Path, ver)
	} else {
		s, err = kv.Get(ctx, v.Path)
	}
	if err != nil {
		return "", err
	}

	val, ok := s.Data[v.Key].(string)
	if !ok {
		return "", fmt.Errorf("key %q missing or not a string in %s/%s", v.Key, v.Mount, v.Path)
	}
	return val, nil
}

func init() {
	Register("vault", func() Provider { return &Vault{Mount: "kv", Ref: Ref{Version: "latest"}} })
}
