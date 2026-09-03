package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type FanProfile struct {
	Percent          int  `json:"percent"`
	RestoreOnStartup bool `json:"restore_on_startup"`
}

type Config struct {
	Aliases     map[string]string     `json:"aliases"`
	Profiles    map[string]int        `json:"profiles"`
	FanProfiles map[string]FanProfile `json:"fan_profiles,omitempty"`
	LastProfile string                `json:"last_profile,omitempty"`
	MinPercent  int                   `json:"min_percent"`
}

type ConfigStore struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func defaultConfig() Config {
	return Config{
		Aliases:     map[string]string{},
		FanProfiles: map[string]FanProfile{},
		Profiles: map[string]int{
			"quiet":       40,
			"balanced":    55,
			"performance": 75,
			"full":        100,
		},
		MinPercent: 30,
	}
}

func NewConfigStore(path string) (*ConfigStore, error) {
	s := &ConfigStore{path: path, cfg: defaultConfig()}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ConfigStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s.saveLocked()
	}
	if err != nil {
		return err
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return err
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}
	if cfg.Profiles == nil {
		cfg.Profiles = defaultConfig().Profiles
	}
	if cfg.FanProfiles == nil {
		cfg.FanProfiles = map[string]FanProfile{}
	}
	if cfg.MinPercent < 1 || cfg.MinPercent > 100 {
		cfg.MinPercent = 30
	}
	s.cfg = cfg
	return nil
}

func (s *ConfigStore) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := s.cfg
	out.Aliases = make(map[string]string, len(s.cfg.Aliases))
	for k, v := range s.cfg.Aliases {
		out.Aliases[k] = v
	}
	out.Profiles = make(map[string]int, len(s.cfg.Profiles))
	for k, v := range s.cfg.Profiles {
		out.Profiles[k] = v
	}
	out.FanProfiles = make(map[string]FanProfile, len(s.cfg.FanProfiles))
	for k, v := range s.cfg.FanProfiles {
		out.FanProfiles[k] = v
	}
	return out
}

func (s *ConfigStore) SetAlias(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		delete(s.cfg.Aliases, id)
	} else {
		s.cfg.Aliases[id] = name
	}
	return s.saveLocked()
}

func (s *ConfigStore) SetFanProfile(id string, percent int, restoreOnStartup bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if percent < s.cfg.MinPercent || percent > 100 {
		return errors.New("fan profile percentage outside configured safety range")
	}
	if s.cfg.FanProfiles == nil {
		s.cfg.FanProfiles = map[string]FanProfile{}
	}
	s.cfg.FanProfiles[id] = FanProfile{
		Percent:          percent,
		RestoreOnStartup: restoreOnStartup,
	}
	return s.saveLocked()
}

func (s *ConfigStore) DeleteFanProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cfg.FanProfiles, id)
	return s.saveLocked()
}

func (s *ConfigStore) SetLastProfile(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.LastProfile = name
	return s.saveLocked()
}

func (s *ConfigStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
