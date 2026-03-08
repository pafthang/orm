package orm

import (
	"fmt"
	"sort"
	"sync"
)

// Plugin installs cross-cutting behavior (interceptors, observers, etc.).
type Plugin interface {
	Name() string
	Install(host *PluginHost) error
}

// PluginHost exposes extension points available for plugins.
type PluginHost struct{}

func (h *PluginHost) AddBeforeInterceptor(fn BeforeInterceptor) { AddBeforeInterceptor(fn) }
func (h *PluginHost) AddAfterInterceptor(fn AfterInterceptor)   { AddAfterInterceptor(fn) }

var pluginsState struct {
	mu   sync.RWMutex
	list map[string]Plugin
}

// RegisterPlugin installs plugin once by unique name.
func RegisterPlugin(p Plugin) error {
	if p == nil {
		return ErrInvalidQuery.with("register_plugin", "", "", fmt.Errorf("plugin is nil"))
	}
	name := p.Name()
	if name == "" {
		return ErrInvalidQuery.with("register_plugin", "", "", fmt.Errorf("plugin name is empty"))
	}
	pluginsState.mu.Lock()
	defer pluginsState.mu.Unlock()
	if pluginsState.list == nil {
		pluginsState.list = map[string]Plugin{}
	}
	if _, exists := pluginsState.list[name]; exists {
		return ErrConflict.with("register_plugin", "", name, fmt.Errorf("plugin already registered"))
	}
	host := &PluginHost{}
	if err := p.Install(host); err != nil {
		return ErrInvalidQuery.with("register_plugin", "", name, err)
	}
	pluginsState.list[name] = p
	return nil
}

// RegisteredPlugins returns sorted plugin names.
func RegisteredPlugins() []string {
	pluginsState.mu.RLock()
	defer pluginsState.mu.RUnlock()
	out := make([]string, 0, len(pluginsState.list))
	for name := range pluginsState.list {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ResetPlugins clears plugin registry and global interceptor lists.
func ResetPlugins() {
	pluginsState.mu.Lock()
	pluginsState.list = nil
	pluginsState.mu.Unlock()
	ResetInterceptors()
}
