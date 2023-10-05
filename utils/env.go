package utils

import (
	"os"
	"strconv"
	"strings"
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

// LookupEnvAsStringMap returns a map of strings from the environment variable with the given key
// attributes are comma separated, key/value pairs are separated by an equals sign
// Example: key1=value1,key2=value2
func LookupEnvAsStringMap(key string) map[string]string {
	values := make(map[string]string)
	if env := os.Getenv(key); env != "" {
		for _, value := range strings.Split(env, ",") {
			parts := strings.Split(value, "=")
			if len(parts) == 2 {
				values[parts[0]] = parts[1]
			}
		}
	}
	return values
}
