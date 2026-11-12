package utils

import (
	"os"
	"strconv"
	"strings"
)

// GetEnvAsBool parses a boolean environment variable with a default.
func GetEnvAsBool(key string, defaultVal bool) bool {
	val := strings.ToLower(os.Getenv(key))
	switch val {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return defaultVal
	}
}

// Atoi32 parses a decimal integer and rejects values outside the int32 range,
// so the result can be converted to int32 (database limits, offsets and IDs)
// without wrapping.
func Atoi32(s string) (int, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// GetEnvAsInt retrieves an environment variable as an integer with a default fallback.
func GetEnvAsInt(name string, defaultVal int) int {
	if valStr := os.Getenv(name); valStr != "" {
		if val, err := Atoi32(valStr); err == nil {
			return val
		}
	}
	return defaultVal
}

// GetEnvAsFloat retrieves an environment variable as a float64 with a default fallback.
func GetEnvAsFloat(name string, defaultVal float64) float64 {
	if valStr := os.Getenv(name); valStr != "" {
		if val, err := strconv.ParseFloat(valStr, 64); err == nil {
			return val
		}
	}
	return defaultVal
}

// GetEnvAsSlice retrieves an environment variable as a slice of strings, split by a separator.
func GetEnvAsSlice(name string, defaultVal []string, sep string) []string {
	if valStr := os.Getenv(name); valStr != "" {
		return strings.Split(valStr, sep)
	}
	return defaultVal
}
