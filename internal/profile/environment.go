package profile

import (
	"path/filepath"
	"strings"
)

// ReplaceEnvironment returns environment with key set to value, removing any
// previous occurrence.
func ReplaceEnvironment(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, variable := range environment {
		if !strings.HasPrefix(variable, prefix) {
			result = append(result, variable)
		}
	}
	return append(result, prefix+value)
}

// PrivateHomeEnvironment points every home-locating variable a process might
// read at the profile's composed private home.
func PrivateHomeEnvironment(environment []string, profileHome string) []string {
	volume := filepath.VolumeName(profileHome)
	values := []struct {
		key   string
		value string
	}{
		{key: "HOME", value: profileHome},
		{key: "USERPROFILE", value: profileHome},
		{key: "HOMEDRIVE", value: volume},
		{key: "HOMEPATH", value: strings.TrimPrefix(profileHome, volume)},
		{key: "XDG_CONFIG_HOME", value: filepath.Join(profileHome, ".config")},
		{key: "XDG_CACHE_HOME", value: filepath.Join(profileHome, ".cache")},
		{key: "XDG_DATA_HOME", value: filepath.Join(profileHome, ".local", "share")},
		{key: "XDG_STATE_HOME", value: filepath.Join(profileHome, ".local", "state")},
		{key: "APPDATA", value: filepath.Join(profileHome, "AppData", "Roaming")},
		{key: "LOCALAPPDATA", value: filepath.Join(profileHome, "AppData", "Local")},
	}
	for _, variable := range values {
		environment = ReplaceEnvironment(environment, variable.key, variable.value)
	}
	return environment
}
