// Package config holds the few environment-driven defaults pscription has.
//
// Every variable is namespaced PSCRIPTION_*. They used to be bare names
// (VIEW_WIDTH, VIEW_HEIGHT, …) read after loading a .env from the working
// directory, so running pscribe inside ANY project whose .env happened to
// define VIEW_WIDTH changed the capture viewport. Nothing loads a .env any
// more; set these in the environment.
package config

import (
	"os"
	"strconv"
)

var (
	// ViewWidth and ViewHeight are the default viewport, in CSS pixels.
	ViewWidth  int64 = 1600
	ViewHeight int64 = 900

	// ProfileScripts logs each embedded script's execution time.
	ProfileScripts bool
)

// init makes env-driven config work for library consumers too — importing
// any pscription package is enough; no explicit Init call required.
func init() {
	ViewWidth = envToInt64("PSCRIPTION_VIEW_WIDTH", ViewWidth)
	ViewHeight = envToInt64("PSCRIPTION_VIEW_HEIGHT", ViewHeight)
	ProfileScripts = envToBool("PSCRIPTION_PROFILE_SCRIPTS", false)
}

func envToInt64(env string, def int64) int64 {
	val := os.Getenv(env)
	if val == "" {
		return def
	}
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil || i <= 0 {
		return def
	}
	return i
}

func envToBool(env string, def bool) bool {
	val := os.Getenv(env)
	if val == "" {
		return def
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return def
	}
	return b
}
