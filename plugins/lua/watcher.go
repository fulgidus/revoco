package lua

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// PluginWatcher watches plugin files for changes and triggers reloads.
type PluginWatcher struct {
	runtime    *Runtime
	watcher    *fsnotify.Watcher
	plugins    map[string]*LuaPlugin // path -> plugin
	mu         sync.RWMutex
	debounce   map[string]*time.Timer
	debounceMu sync.Mutex

	// Callbacks
	onReload func(plugin *LuaPlugin, err error)
	onError  func(err error)

	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

// WatcherConfig holds configuration for the plugin watcher.
type WatcherConfig struct {
	// DebounceDelay is the delay before reloading after a file change.
	// This prevents multiple reloads during rapid saves.
	DebounceDelay time.Duration
}

// DefaultWatcherConfig returns the default watcher configuration.
func DefaultWatcherConfig() WatcherConfig {
	return WatcherConfig{
		DebounceDelay: 200 * time.Millisecond,
	}
}

// NewPluginWatcher creates a new plugin file watcher.
func NewPluginWatcher(runtime *Runtime) (*PluginWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	pw := &PluginWatcher{
		runtime:  runtime,
		watcher:  watcher,
		plugins:  make(map[string]*LuaPlugin),
		debounce: make(map[string]*time.Timer),
		ctx:      ctx,
		cancel:   cancel,
	}

	return pw, nil
}

// SetOnReload sets the callback for plugin reload events.
func (pw *PluginWatcher) SetOnReload(fn func(plugin *LuaPlugin, err error)) {
	pw.onReload = fn
}

// SetOnError sets the callback for watcher errors.
func (pw *PluginWatcher) SetOnError(fn func(err error)) {
	pw.onError = fn
}

// Watch starts watching a plugin file for changes.
func (pw *PluginWatcher) Watch(plugin *LuaPlugin) error {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	path := plugin.path

	// Add to tracked plugins
	pw.plugins[path] = plugin

	// Watch the file's directory (fsnotify doesn't reliably watch individual files)
	dir := filepath.Dir(path)
	if err := pw.watcher.Add(dir); err != nil {
		return err
	}

	return nil
}

// Unwatch stops watching a plugin file.
func (pw *PluginWatcher) Unwatch(plugin *LuaPlugin) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	delete(pw.plugins, plugin.path)

	// We don't remove the directory watch because other plugins might be there
}

// Start begins processing file system events.
func (pw *PluginWatcher) Start() {
	go pw.eventLoop()
}

// Stop stops the watcher.
func (pw *PluginWatcher) Stop() error {
	pw.cancel()
	return pw.watcher.Close()
}

// eventLoop processes file system events.
func (pw *PluginWatcher) eventLoop() {
	config := DefaultWatcherConfig()

	for {
		select {
		case <-pw.ctx.Done():
			return

		case event, ok := <-pw.watcher.Events:
			if !ok {
				return
			}

			// Only care about write events
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			// Check if this is a watched plugin file
			pw.mu.RLock()
			plugin, exists := pw.plugins[event.Name]
			pw.mu.RUnlock()

			if !exists {
				continue
			}

			// Debounce the reload
			pw.scheduleReload(plugin, config.DebounceDelay)

		case err, ok := <-pw.watcher.Errors:
			if !ok {
				return
			}
			if pw.onError != nil {
				pw.onError(err)
			}
		}
	}
}

// scheduleReload schedules a debounced plugin reload.
func (pw *PluginWatcher) scheduleReload(plugin *LuaPlugin, delay time.Duration) {
	pw.debounceMu.Lock()
	defer pw.debounceMu.Unlock()

	path := plugin.path

	// Cancel existing timer
	if timer, exists := pw.debounce[path]; exists {
		timer.Stop()
	}

	// Schedule new reload
	pw.debounce[path] = time.AfterFunc(delay, func() {
		pw.reloadPlugin(plugin)
	})
}

// reloadPlugin reloads a plugin and calls the callback.
func (pw *PluginWatcher) reloadPlugin(plugin *LuaPlugin) {
	err := plugin.Reload(pw.ctx)

	if pw.onReload != nil {
		pw.onReload(plugin, err)
	}
}

// WatchedPlugins returns the list of currently watched plugins.
func (pw *PluginWatcher) WatchedPlugins() []*LuaPlugin {
	pw.mu.RLock()
	defer pw.mu.RUnlock()

	plugins := make([]*LuaPlugin, 0, len(pw.plugins))
	for _, p := range pw.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// PluginManager extends Runtime with hot-reload capabilities.
type PluginManager struct {
	*Runtime
	watcher *PluginWatcher
	plugins map[string]*LuaPlugin
	mu      sync.RWMutex
}

// NewPluginManager creates a new plugin manager with hot-reload support.
func NewPluginManager() (*PluginManager, error) {
	runtime := NewRuntime()

	watcher, err := NewPluginWatcher(runtime)
	if err != nil {
		return nil, err
	}

	pm := &PluginManager{
		Runtime: runtime,
		watcher: watcher,
		plugins: make(map[string]*LuaPlugin),
	}

	// Set up reload callback
	watcher.SetOnReload(func(plugin *LuaPlugin, err error) {
		if err != nil {
			// Plugin reload failed, update state
			plugin.info.State = "error"
			plugin.info.StateError = err.Error()
		}
	})

	return pm, nil
}

// LoadAndWatch loads a plugin and starts watching it for changes.
func (pm *PluginManager) LoadAndWatch(ctx context.Context, path string) (*LuaPlugin, error) {
	plugin, err := pm.Runtime.LoadPlugin(ctx, path)
	if err != nil {
		return nil, err
	}

	pm.mu.Lock()
	pm.plugins[path] = plugin
	pm.mu.Unlock()

	// Start watching
	if err := pm.watcher.Watch(plugin); err != nil {
		// Non-fatal, just log
	}

	return plugin, nil
}

// Start starts the plugin manager's file watcher.
func (pm *PluginManager) Start() {
	pm.watcher.Start()
}

// Stop stops the plugin manager.
func (pm *PluginManager) Stop() error {
	return pm.watcher.Stop()
}

// GetPlugin returns a loaded plugin by path.
func (pm *PluginManager) GetPlugin(path string) (*LuaPlugin, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plugin, exists := pm.plugins[path]
	return plugin, exists
}

// AllPlugins returns all loaded plugins.
func (pm *PluginManager) AllPlugins() []*LuaPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plugins := make([]*LuaPlugin, 0, len(pm.plugins))
	for _, p := range pm.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// UnloadPlugin unloads a plugin and stops watching it.
func (pm *PluginManager) UnloadPlugin(path string) error {
	pm.mu.Lock()
	plugin, exists := pm.plugins[path]
	if exists {
		delete(pm.plugins, path)
	}
	pm.mu.Unlock()

	if !exists {
		return nil
	}

	pm.watcher.Unwatch(plugin)
	return plugin.Unload()
}
