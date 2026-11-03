package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sylphe88/secompose/internal/backend"
)

// Provide a registry to register providers in main
var providers = map[string]func() backend.Provider{}

func Register(name string, f func() backend.Provider) { providers[name] = f }

// Native keys `file` and `environment`, and custom/dynamic provider
type Secret struct {
	File         string           `yaml:"file"`
	Environment  string           `yaml:"environment"`
	ProviderName string           `yaml:"-"`
	Provider     backend.Provider `yaml:"-"`
	Value        string           `yaml:"-"`
}

func (s *Secret) UnmarshalYAML(n *yaml.Node) error {
	// Decode a plain Secret without method (no unmarshal) to avoid recursion
	type plain Secret
	if err := n.Decode((*plain)(s)); err != nil {
		return err
	}

	for i := 0; i < len(n.Content); i += 2 {
		// Look for one key named x-secompose-* to load the associated provider.
		// No multi-provider allowed. No file+environment allowed.
		name, ok := strings.CutPrefix(n.Content[i].Value, "x-secompose-")
		if !ok {
			continue
		}
		if s.Provider != nil {
			return fmt.Errorf("multiple providers: %s, %s", s.ProviderName, name)
		}

		p, err := backend.Decode(name, n.Content[i+1])
		if err != nil {
			return fmt.Errorf("x-secompose-%s: %w", name, err)
		}
		s.ProviderName, s.Provider = name, p
	}
	switch {
	case s.Provider == nil:
		return errors.New("an x-secompose-* provider is required")
	case s.File != "" && s.Environment != "":
		return errors.New("file and environment are mutually exclusive")
	}
	return nil
}

func (s *Secret) Resolve(ctx context.Context) error {
	v, err := s.Provider.Load(ctx)
	if err == nil {
		s.Value = v
		return nil
	}
	fmt.Println("Error while resolving secret, failing over...")
	switch {
	case s.Environment != "":
		if v, ok := os.LookupEnv(s.Environment); ok {
			s.Value = v
			return nil
		}
	case s.File != "":
		if b, ferr := os.ReadFile(s.File); ferr == nil {
			s.Value = strings.TrimRight(string(b), "\n")
			return nil
		}
	}
	return fmt.Errorf("%s: %w", s.ProviderName, err)
}

type Secrets map[string]*Secret

func (ss *Secrets) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("secrets: expected a mapping")
	}
	*ss = Secrets{}
	for i := 0; i < len(n.Content); i += 2 {
		name := n.Content[i].Value
		var s Secret
		if err := n.Content[i+1].Decode(&s); err != nil {
			return fmt.Errorf("secret %q: %w", name, err)
		}
		(*ss)[name] = &s
	}
	return nil
}

type Compose struct {
	Secrets Secrets `yaml:"secrets"`
}

func ParseSecrets(path string) (Secrets, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Compose
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return c.Secrets, nil
}

func (ss Secrets) ResolveAll(ctx context.Context) error {
	for name, s := range ss {
		if err := s.Resolve(ctx); err != nil {
			return fmt.Errorf("secret %q didn't resolve: %w", name, err)
		}
	}
	return nil
}
