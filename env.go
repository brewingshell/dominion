package main

import (
	"os"
	"path/filepath"
	"strings"
)

// defaultPINValue is the portal PIN used when no flag, environment variable, or
// .env file supplies one.
const defaultPINValue = "1111"

// envFileName is the dotenv file the portal reads its configuration from.
const envFileName = ".env"

// pinFromEnv resolves the portal PIN with this precedence:
//
//  1. the DOMINION_PIN environment variable
//  2. DOMINION_PIN in a .env file, searched as $DOMINION_ENV, then ./.env, then
//     ~/.config/dominion/.env
//  3. defaultPINValue
//
// An explicit -pin flag overrides all of it; flag defaults are seeded from here.
func pinFromEnv(home string) string {
	if v := os.Getenv("DOMINION_PIN"); v != "" {
		return v
	}
	for _, path := range envFileCandidates(home) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if v := parseEnv(string(data))["DOMINION_PIN"]; v != "" {
			return v
		}
	}
	return defaultPINValue
}

// envFileCandidates lists the .env locations to try, most specific first.
func envFileCandidates(home string) []string {
	var paths []string
	if p := os.Getenv("DOMINION_ENV"); p != "" {
		paths = append(paths, p)
	}
	paths = append(paths, envFileName)
	if home != "" {
		paths = append(paths, filepath.Join(home, ".config", "dominion", envFileName))
	}
	return paths
}

// parseEnv parses dotenv content: KEY=VALUE per line, blank lines and # comments
// ignored, an optional "export " prefix tolerated, and surrounding single or
// double quotes stripped.
func parseEnv(content string) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		values[key] = unquote(strings.TrimSpace(value))
	}
	return values
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
