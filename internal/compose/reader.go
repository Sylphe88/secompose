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
		return errors.New("A x-secompose-* provider is required")
	case s.Environment == "":
		return errors.New("`environment` must also be set to hydrate in-memory compose after secret resolution")
	case s.File != "" && s.Environment != "":
		return errors.New("file and environment are mutually exclusive")
	}
	return nil
}

func (s *Secret) Resolve(ctx context.Context) {
	v, err := s.Provider.Load(ctx)
	if err == nil {
		s.Value = v
	} else {
		fmt.Println("secompose: secret provider failed to resolve. Failing over to env.")
		fmt.Printf("secompose: provider details - %s: %v\n", s.ProviderName, err)
	}

	if v, found := os.LookupEnv(s.Environment); found {
		s.Value = v
	} else {
		s.Value = ""
	}
}

type Secrets map[string]*Secret

func envName(name string, s *Secret) string {
	if s.Environment != "" {
		return s.Environment
	}
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func ToEnvKeyValue(name string, s *Secret) string {
	return envName(name, s) + "=" + s.Value
}

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

func ParseSecrets(composeData []byte) (Secrets, error) {

	var c Compose
	if err := yaml.Unmarshal(composeData, &c); err != nil {
		return nil, err
	}
	return c.Secrets, nil
}

func (ss Secrets) ResolveAll(ctx context.Context) error {
	for name, s := range ss {
		s.Resolve(ctx)
		if s.Value == "" {
			return fmt.Errorf("secret %q resolved to an empty value", name)
		}
	}
	return nil
}
