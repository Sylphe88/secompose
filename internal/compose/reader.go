package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Ref struct {
	Key string `yaml:"key"`
}

func (r *Ref) ref() *Ref { return r }

type Provider interface {
	ref() *Ref
	Resolve(ctx context.Context) (string, error)
}

// Provide a registry to register providers in main
var providers = map[string]func() Provider{}

func Register(name string, f func() Provider) { providers[name] = f }

// Native keys `file` and `environment`, and custom/dynamic provider
type Secret struct {
	File         string   `yaml:"file"`
	Environment  string   `yaml:"environment"`
	ProviderName string   `yaml:"-"`
	Provider     Provider `yaml:"-"`
	Value        string   `yaml:"-"`
}

func (s *Secret) Load(ctx context.Context) error {
	v, err := s.Provider.Resolve(ctx)
	if err == nil {
		s.Value = v
		return nil
	}
	return fmt.Errorf("%s: %w", s.ProviderName, err)
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
		newProvider, ok := providers[name]
		if !ok {
			return fmt.Errorf("unknown provider %q", name)
		}
		p := newProvider()
		if err := n.Content[i+1].Decode(p); err != nil {
			return fmt.Errorf("x-secompose-%s: %w", name, err)
		}
		if r := p.ref(); r.Key == "" {
			return fmt.Errorf("x-secompose-%s: key is required", name)
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

func parseSecrets(path string) (Secrets, error) {
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
