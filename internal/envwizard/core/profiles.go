package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

type Profile struct {
	Name   string            `json:"name"`
	Values map[string]string `json:"values"`
}

func profileDir() (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, ".config")
	}
	dir := filepath.Join(configDir, "patreon-manager", "profiles")
	return dir, os.MkdirAll(dir, 0700)
}

func profilePath(name string) (string, error) {
	dir, err := profileDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

func SaveProfile(p *Profile) error {
	path, err := profilePath(p.Name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func LoadProfile(name string) (*Profile, error) {
	path, err := profilePath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func ListProfiles() ([]string, error) {
	dir, err := profileDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	sort.Strings(names)
	return names, nil
}

func DeleteProfile(name string) error {
	path, err := profilePath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}
