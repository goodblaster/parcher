// Package config loads parch's layered configuration: built-in defaults, a
// global ~/.parch/config, a project-local ./.parch/config (found by walking up
// from the working directory), and finally CLI flags — each layer overriding
// the one before it. The file format is TOML.
//
// Example ~/.parch/config:
//
//	cache_dir  = "~/.parch/cache"
//	output_dir = "~/archives"
//	filename   = "{host}-{date}.{ext}"
//
//	[defaults]
//	format     = "html"
//	width      = 1600
//	height     = 900
//	dpr        = 1
//	links      = "keep"
//	favicon    = true
//	text       = true
//	user_agent = ""        # empty = the browser's own Chrome user agent
package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the merged parch configuration read from TOML files. Zero-valued
// fields mean "not set" and let a lower-precedence layer (or a built-in
// default) show through.
type Config struct {
	// CacheDir is Chrome's shared HTTP cache directory (speeds up repeat
	// loads). Empty disables the shared cache.
	CacheDir string `toml:"cache_dir"`
	// OutputDir is where captures are written when -o is not an explicit path.
	OutputDir string `toml:"output_dir"`
	// Filename is the output filename template (see filename.go for tokens).
	Filename string `toml:"filename"`
	// Defaults supply per-flag defaults, overridden by explicit CLI flags.
	Defaults Defaults `toml:"defaults"`
}

// Defaults mirrors the parch flags that can be defaulted from config.
type Defaults struct {
	Format    string  `toml:"format"`
	Width     int     `toml:"width"`
	Height    int     `toml:"height"`
	DPR       float64 `toml:"dpr"`
	Paper     string  `toml:"paper"`
	Links     string  `toml:"links"`
	Timeout   int     `toml:"timeout"`
	Profile   string  `toml:"profile"`
	UserAgent string  `toml:"user_agent"`
	Favicon   *bool   `toml:"favicon"`
	Text      *bool   `toml:"text"`
}

// ConfigDirName is the directory that holds a project-local or global config.
const ConfigDirName = ".parch"

// configFileName is the file inside ConfigDirName.
const configFileName = "config"

// Load reads and merges the global (~/.parch/config) then project-local
// (./.parch/config, nearest ancestor) config files. Later files override
// earlier ones field-by-field: because each file is decoded onto the same
// struct, a key absent from the local file keeps the global value. A missing
// file is not an error. It also returns the paths actually loaded, for
// diagnostics, and a warning for every key no field claims — a typo'd key
// ("widht = 1280") used to be ignored in silence, leaving the default in
// force with nothing to say why.
func Load() (cfg Config, loaded, warnings []string, err error) {
	for _, path := range configPaths() {
		if _, statErr := os.Stat(path); statErr != nil {
			continue // missing file: skip
		}
		meta, decErr := toml.DecodeFile(path, &cfg)
		if decErr != nil {
			return cfg, loaded, warnings, decErr
		}
		for _, key := range meta.Undecoded() {
			warnings = append(warnings, path+": unknown key "+key.String())
		}
		loaded = append(loaded, path)
	}
	return cfg, loaded, warnings, nil
}

// configPaths returns the config files to try, lowest precedence first:
// the global config, then the nearest project-local config. Run from
// anywhere under the home directory, the upward walk for a local config
// ARRIVES at the global one — the same file must not be decoded twice.
func configPaths() []string {
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ConfigDirName, configFileName))
	}
	if local := findLocalConfig(); local != "" && (len(paths) == 0 || local != paths[0]) {
		paths = append(paths, local)
	}
	return paths
}

// findLocalConfig walks up from the working directory looking for a
// .parch/config file, returning the first (nearest) one found.
func findLocalConfig() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ConfigDirName, configFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached filesystem root
		}
		dir = parent
	}
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
