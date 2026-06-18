package config

import (
	"encoding/json"
	"os"
)

// readJSON unmarshals the JSON file at path into a value of type T. A missing
// file is not an error: it yields the zero value of T (a nil slice, a zero
// struct), which callers treat as "nothing saved yet".
func readJSON[T any](path string) (T, error) {
	var v T
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return v, nil
		}
		return v, err
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return v, err
	}
	return v, nil
}

// writeJSON writes v as indented JSON to path, creating the config dir first.
func writeJSON[T any](path string, v T) error {
	if err := EnsureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
