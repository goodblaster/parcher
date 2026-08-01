package config

import (
	"os"
	"strconv"
)

var (
	MaxHeight   int64
	ViewWidth   int64 = 1600
	ViewHeight  int64 = 900
	DataDir     string
	ProfileJson bool
)

// init makes env-driven config work for library consumers too — importing
// any pscription package is enough; no explicit Init call required.
func init() {
	Init()
}

func Init() {
	ViewWidth = envToInt64("VIEW_WIDTH", ViewWidth)
	ViewHeight = envToInt64("VIEW_HEIGHT", ViewHeight)
	DataDir = os.Getenv("DATA_DIR")
	MaxHeight = envToInt64("MAX_HEIGHT", 0) // 0 in unlimited
	ProfileJson = envToBool("PROFILE_JSON", false)
}

func envToInt64(env string, def int64) int64 {
	val := os.Getenv(env)
	if val == "" {
		return def
	}

	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
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
