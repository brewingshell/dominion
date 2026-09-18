package main

import (
	"os"
	"path/filepath"
	"strings"
)

// defaultPINValue is the portal PIN used when no flag, environment variable,
// .env file, or pin file supplies one.
const defaultPINValue = "1111"

// envFileName is the dotenv file the portal reads its configuration from.
const envFileName = ".env"

// pinFileName is the file the portal writes when the PIN is changed from the
// settings dialog. It outranks both the -pin flag and DOMINION_PIN so a change
// made in the UI is not silently reverted on restart.
const pinFileName = "pin"

// pinFromEnv resolves the portal PIN from the environment, ignoring the pin
// file and the -pin flag, with this precedence:
//
//  1. the DOMINION_PIN environment variable
//  2. DOMINION_PIN in a .env file, searched as $DOMINION_ENV, then ./.env, then
//     ~/.config/dominion/.env
//  3. defaultPINValue
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

// resolvePIN layers the pin file and the explicit -pin flag over the
// environment:
//
//  1. the pin file (pinFile, or <home>/.config/dominion/pin), when present and
//     non-empty — a change made in the settings dialog outranks the operator's
//     flag and environment
//  2. flagValue, when the -pin flag was set
//  3. pinFromEnv (DOMINION_PIN env, then .env, then the default)
func resolvePIN(home, flagValue, pinFile string) string {
	pinFile = effectivePINFile(home, pinFile)
	if v := readPINFile(pinFile); v != "" {
		return v
	}
	if flagValue != "" {
		return flagValue
	}
	return pinFromEnv(home)
}

// effectivePINFile returns pinFile when set, otherwise the default location
// under home. An empty result (no home, no explicit file) disables the pin
// file.
func effectivePINFile(home, pinFile string) string {
	if pinFile != "" {
		return pinFile
	}
	return defaultPINFile(home)
}

// defaultPINFile is where the settings dialog persists a changed PIN.
func defaultPINFile(home string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "dominion", pinFileName)
}

// readPINFile returns the trimmed contents of path, or "" when the file is
// missing, empty, or unreadable. A missing file is the normal case, not an
// error.
func readPINFile(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
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
