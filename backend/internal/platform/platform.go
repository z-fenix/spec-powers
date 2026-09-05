// Package platform abstracts hosted git platforms behind a Provider
// interface. Resource binding and future PR integration speak to "a
// platform" rather than GitHub specifically; GitHub is the only adapter
// this iteration, with Forgejo / Gitea / GitLab expected to register
// their own implementations later.
package platform

import (
	"fmt"
	"sort"
	"sync"
)

// Provider is one hosted git platform. A provider owns the pointer
// syntax of its resource type and the URL shapes derived from it.
type Provider interface {
	// Type is the resource type this provider handles, e.g. "github_repo".
	Type() string
	// ValidatePointer reports whether pointer is a well-formed repo
	// reference for this platform.
	ValidatePointer(pointer string) error
	// CloneURL returns an HTTPS clone URL for the repo.
	CloneURL(pointer string) (string, error)
	// WebURL returns the browse URL for the repo.
	WebURL(pointer string) (string, error)
}

var (
	mu         sync.RWMutex
	byTypeName = map[string]Provider{}
)

// Register makes a provider available under its Type. Registering a
// duplicate type panics: the set of platforms is startup configuration.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := byTypeName[p.Type()]; dup {
		panic(fmt.Sprintf("platform: provider %q already registered", p.Type()))
	}
	byTypeName[p.Type()] = p
}

// ForType returns the provider handling the given resource type.
func ForType(resourceType string) (Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := byTypeName[resourceType]
	return p, ok
}

// Types lists the registered resource types sorted.
func Types() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(byTypeName))
	for t := range byTypeName {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func init() {
	Register(&GitHubProvider{})
}
