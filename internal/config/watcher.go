package config

import (
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	manager  *Manager
	watcher  *fsnotify.Watcher
	filename string
	done     chan struct{}
}

func NewWatcher(manager *Manager, filename string) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		manager:  manager,
		watcher:  watcher,
		filename: filename,
		done:     make(chan struct{}),
	}

	dir := filepath.Dir(filename)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, err
	}

	return w, nil
}

func (w *Watcher) Start() {
	go w.watch()
}

func (w *Watcher) Stop() {
	close(w.done)
	w.watcher.Close()
}

func (w *Watcher) watch() {
	debounce := time.NewTimer(0)
	<-debounce.C

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			if filepath.Clean(event.Name) == filepath.Clean(w.filename) {
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					debounce.Stop()
					debounce = time.NewTimer(100 * time.Millisecond)
					
					go func() {
						<-debounce.C
						log.Printf("Configuration file changed, reloading...")
						if err := w.manager.Load(w.filename); err != nil {
							log.Printf("Failed to reload configuration: %v", err)
						}
					}()
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)

		case <-w.done:
			return
		}
	}
}