package mdloader

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Watcher uses fsnotify to watch the agents, policies, and playbooks
// directories recursively. On file change it invalidates the corresponding
// cache entry in the Loader and fires any registered callbacks.
type Watcher struct {
	fsWatcher *fsnotify.Watcher
	loader    *Loader
	callbacks []func(path string, op string)
	mu        sync.Mutex // protects callbacks slice
	done      chan struct{}
	logger    *slog.Logger
}

// NewWatcher creates a Watcher that will watch the three directories
// configured in the given Loader. Call Start() to begin processing events.
func NewWatcher(loader *Loader, logger *slog.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if logger == nil {
		logger = slog.Default()
	}

	w := &Watcher{
		fsWatcher: fsw,
		loader:    loader,
		done:      make(chan struct{}),
		logger:    logger.With("component", "mdloader.Watcher"),
	}

	// Register ourselves on the loader so it knows it has a watcher.
	loader.SetWatcher(w)

	// Walk all three directories and add them (+ subdirs) to fsnotify.
	dirs := []string{loader.AgentsDir(), loader.PoliciesDir(), loader.PlaybooksDir()}
	for _, dir := range dirs {
		if err := w.addRecursive(dir); err != nil {
			// Non-fatal: directory may not exist yet at startup.
			w.logger.Warn("could not watch directory",
				"dir", dir,
				"error", err,
			)
		}
	}

	return w, nil
}

// OnChange registers a callback that is invoked whenever a watched file
// changes. The op string is one of "create", "write", "remove", "rename".
// Callbacks are invoked synchronously on the watcher goroutine; keep them
// fast or dispatch to another goroutine.
func (w *Watcher) OnChange(fn func(path string, op string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, fn)
}

// Start begins watching for filesystem events in a background goroutine.
// It returns immediately. Call Stop() to shut down.
func (w *Watcher) Start() error {
	go w.loop()
	return nil
}

// Stop shuts down the watcher and releases resources.
func (w *Watcher) Stop() error {
	close(w.done)
	return w.fsWatcher.Close()
}

// loop is the main event processing goroutine.
func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("fsnotify error", "error", err)
		}
	}
}

// handleEvent processes a single fsnotify event.
func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name
	op := opString(event.Op)

	// If a new directory was created, start watching it too.
	if event.Op.Has(fsnotify.Create) {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			if err := w.addRecursive(path); err != nil {
				w.logger.Warn("failed to watch new directory",
					"path", path,
					"error", err,
				)
			}
		}
	}

	// Only process .md file events.
	if filepath.Ext(path) != ".md" {
		return
	}

	w.logger.Debug("md file changed",
		"path", path,
		"op", op,
	)

	// Invalidate the loader cache for this file.
	w.loader.Invalidate(path)

	// Fire callbacks.
	w.mu.Lock()
	cbs := make([]func(string, string), len(w.callbacks))
	copy(cbs, w.callbacks)
	w.mu.Unlock()

	for _, fn := range cbs {
		fn(path, op)
	}
}

// addRecursive walks a directory tree and adds every directory to the
// fsnotify watcher.
func (w *Watcher) addRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip inaccessible paths rather than aborting the walk.
			return nil
		}
		if info.IsDir() {
			if err := w.fsWatcher.Add(path); err != nil {
				w.logger.Warn("failed to add directory to watcher",
					"path", path,
					"error", err,
				)
			}
		}
		return nil
	})
}

// opString converts an fsnotify.Op bitmask to a human-readable string.
func opString(op fsnotify.Op) string {
	switch {
	case op.Has(fsnotify.Create):
		return "create"
	case op.Has(fsnotify.Write):
		return "write"
	case op.Has(fsnotify.Remove):
		return "remove"
	case op.Has(fsnotify.Rename):
		return "rename"
	case op.Has(fsnotify.Chmod):
		return "chmod"
	default:
		return op.String()
	}
}
