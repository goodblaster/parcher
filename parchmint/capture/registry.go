package capture

import "sort"

// Backends register themselves under the format name that selects them, in
// their own file's init. That is the point of the registry rather than a
// switch in the CLI: a backend can be added or REMOVED by adding or deleting
// one file, with no edit anywhere else. It was built for exactly that: the
// AGPL-licensed SingleFile backend lived behind a build tag until the native
// HTML backend was trusted, and retiring it was deleting its file — no CLI
// edit, no dangling format name. Keep that property.
var (
	backends = map[string]func() Backend{}
	aliases  = map[string]string{}
)

// Register makes a backend selectable as -f format. Registering the same
// format twice is a programming error and panics, since it would silently
// make the winner depend on init order.
func Register(format string, newBackend func() Backend) {
	if _, dup := backends[format]; dup {
		panic("capture: duplicate backend format " + format)
	}
	backends[format] = newBackend
}

// RegisterAlias makes alias select the backend registered for format. An
// alias is not listed by Formats — it is a spelling, not a choice.
func RegisterAlias(alias, format string) {
	aliases[alias] = format
}

// Lookup returns a new backend for the given format name, or nil when the
// format is not registered.
func Lookup(format string) Backend {
	if target, ok := aliases[format]; ok {
		format = target
	}
	newBackend, ok := backends[format]
	if !ok {
		return nil
	}
	return newBackend()
}

// Formats lists the registered format names, sorted, for help text and error
// messages. Building those from the registry is what keeps them honest in a
// build where a backend is compiled out.
func Formats() []string {
	names := make([]string, 0, len(backends))
	for name := range backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LayerCarrier marks a backend whose output is a document container — one
// that can carry the embedded text layer and inlined favicons. The raster and
// PDF backends produce pictures of a page and cannot.
//
// It is a marker interface rather than a list of format names in the CLI so
// that the knowledge lives with the backend: deleting a backend's file
// deletes its answer too.
type LayerCarrier interface {
	CarriesLayer()
}

// CarriesLayer reports whether backend's container can embed the text layer
// and favicons.
func CarriesLayer(backend Backend) bool {
	_, ok := backend.(LayerCarrier)
	return ok
}
