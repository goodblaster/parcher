package common

import (
	"os"

	"github.com/goodblaster/logos"
	"github.com/goodblaster/parcher/pscription/internal/config"
	"github.com/joho/godotenv"
)

// Init sets up logging, config, and environment for all CLI tools
func Init() error {
	// Load .env file (ignore error if it doesn't exist)
	_ = godotenv.Load("./cli/.env")
	_ = godotenv.Load(".env")

	// Set up logging. Logs go to stderr so tool output can be piped from stdout.
	logger := logos.NewLogger(logos.LevelDebug, logos.ConsoleFormatter(), os.Stderr)
	logos.SetDefaultLogger(logger)

	// Initialize configuration
	config.Init()

	return nil
}
