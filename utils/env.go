package utils

import (
	"os"
	"strconv"
)

// LookupEnvOrBool returns a bool parsed from the environment variable with the given key
// or the default value if the environment variable is not set or cannot be parsed as a bool
func LookupEnvOrBool(key string, def bool) bool {
	if env := os.Getenv(key); env != "" {
		if b, err := strconv.ParseBool(env); err == nil {
			return b
		}
	}
	return def
}

// LookupEnvOrString returns a string from the environment variable with the given key
// or the default value if the environment variable is not set
func LookupEnvOrString(key string, def string) string {
	if env := os.Getenv(key); env != "" {
		return env
	}
	return def
}
