// A simple and consistent method to extract a configuration from a file.
// This doesn't contain any method to actually decode the file. The actual
// decoding is done in an external function.
package config

import (
	"bitbucket.org/kardianos/osext"
	"code.google.com/p/go.exp/fsnotify"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

const DefaultPostfix = "_config.json"

// Simple JSON based configuration decoder.
func DecodeJsonConfig(r io.Reader, v interface{}) error {
	d := json.NewDecoder(r)
	return d.Decode(v)
}

// Return a configuration file path. If baseName is empty,
// then the current executable name without the extension
// is used. If postfix is empty then DefaultPostfix is used.
func GetConfigFilePath(baseName, postfix string) (string, error) {
	if len(postfix) == 0 {
		postfix = DefaultPostfix
	}
	path, err := osext.Executable()
	if err != nil {
		return "", err
	}
	path, exeName := filepath.Split(path)
	if len(baseName) == 0 {
		exeName = exeName[:len(exeName)-len(filepath.Ext(exeName))]
	} else {
		exeName = baseName
	}
	configPath := filepath.Join(path, exeName+postfix)
	return configPath, nil
}

type DecodeConfig func(r io.Reader, v interface{}) error
type EncodeConfig func(w io.Writer, v interface{}) error

type WatchConfig struct {
	// Notified here if the file changes.
	C chan *WatchConfig

	filepath string
	watch    *fsnotify.Watcher
	decode   DecodeConfig

	close chan struct{}
}

// Create a new configuration watcher. Adds a notification if the configuration file changes
// so it may be reloaded. If defaultConfig is not nil and encode is not nil, the
// configuration file path is checked if a file exists or not. If it doesn't exist
// the default configuration is written to the file.
func NewWatchConfig(filepath string, decode DecodeConfig, defaultConfig interface{}, encode EncodeConfig) (*WatchConfig, error) {
	if defaultConfig != nil && encode != nil {
		f, err := os.Open(filepath)
		if f != nil {
			f.Close()
		}
		if os.IsNotExist(err) {
			f, err = os.Create(filepath)
			if err != nil {
				return nil, err
			}
			err = encode(f, defaultConfig)
			f.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	watch, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	err = watch.WatchFlags(filepath, fsnotify.FSN_MODIFY|fsnotify.FSN_CREATE)
	if err != nil {
		return nil, err
	}

	wc := &WatchConfig{
		C: make(chan *WatchConfig),

		filepath: filepath,
		watch:    watch,
		decode:   decode,

		close: make(chan struct{}, 0),
	}
	go wc.run()

	return wc, nil
}

func (wc *WatchConfig) run() {
	for {
		select {
		case <-wc.close:
			return
		case <-wc.watch.Event:
			wc.C <- wc
		}
	}
}

// Send a notification as if the configuration file has changed.
func (wc *WatchConfig) TriggerC() {
	wc.C <- wc
}

// Load the configuration from the file into the provided value.
func (wc *WatchConfig) Load(v interface{}) error {
	f, err := os.Open(wc.filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	return wc.decode(f, v)
}

// Stop the watch and any loops that are running.
func (wc *WatchConfig) Close() error {
	wc.close <- struct{}{}
	return wc.watch.Close()
}
