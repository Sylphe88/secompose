package backend

import "fmt"

// factory is a function that constructs a Backend, reading configuration
// (credentials, addresses) from the environment.
type factory func() (Backend, error)

// registry maps backend names to their factory functions.
var registry = map[string]factory{
	"vault": func() (Backend, error) { return NewVault() },
}

// Get returns an initialised Backend for the given name, or an error if the
// name is unknown or the backend fails to initialise (e.g. missing env vars).
func Get(name string) (Backend, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q (available: vault)", name)
	}
	return f()
}
