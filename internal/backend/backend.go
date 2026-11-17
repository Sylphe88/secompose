package backend

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type Ref struct {
	Path    string `yaml:"path"`
	Key     string `yaml:"key"`
	Version string `yaml:"version"`
}

func (r *Ref) ref() *Ref { return r }

type Provider interface {
	ref() *Ref
	Load(ctx context.Context) (string, error)
}

var registry = map[string]func() Provider{}

func Register(name string, f func() Provider) { registry[name] = f }

func Decode(name string, n *yaml.Node) (Provider, error) {
	newProvider, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	p := newProvider()
	if err := n.Decode(p); err != nil {
		return nil, err
	}
	if r := p.ref(); r.Key == "" {
		return nil, errors.New("path and key are required")
	}
	return p, nil
}
