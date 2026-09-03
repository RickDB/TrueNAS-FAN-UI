package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	fanInputRE = regexp.MustCompile(`^fan([0-9]+)_input$`)
	pwmRE      = regexp.MustCompile(`^pwm([0-9]+)$`)
	tempRE     = regexp.MustCompile(`^temp([0-9]+)_input$`)
)

type Fan struct {
	ID             string `json:"id"`
	Index          int    `json:"index"`
	Name           string `json:"name"`
	DefaultName    string `json:"default_name"`
	Chip           string `json:"chip"`
	Device         string `json:"device"`
	RPM            *int   `json:"rpm,omitempty"`
	PWM            *int   `json:"pwm,omitempty"`
	Percent        *int   `json:"percent,omitempty"`
	PWMMode        *int   `json:"pwm_mode,omitempty"`
	PWMPath        string `json:"-"`
	PWMEnablePath  string `json:"-"`
	Writable       bool   `json:"writable"`
	ModeWritable   bool   `json:"mode_writable"`
	CanRestoreMode bool   `json:"can_restore_mode"`
}

type Temperature struct {
	Chip    string  `json:"chip"`
	Label   string  `json:"label"`
	Celsius float64 `json:"celsius"`
}

type Snapshot struct {
	Fans         []Fan         `json:"fans"`
	Temperatures []Temperature `json:"temperatures"`
}

type FanManager struct {
	root          string
	cfg           *ConfigStore
	mu            sync.Mutex
	originalModes map[string]int
}

func NewFanManager(root string, cfg *ConfigStore) *FanManager {
	return &FanManager{root: root, cfg: cfg, originalModes: map[string]int{}}
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readInt(path string) (*int, error) {
	s, err := readText(path)
	if err != nil {
		return nil, err
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func fileWritable(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return st.Mode().Perm()&0o222 != 0
}

func stableDeviceIdentity(hw string, root string) string {
	device := filepath.Join(hw, "device")
	if p, err := filepath.EvalSymlinks(device); err == nil {
		return strings.TrimPrefix(p, root)
	}
	if p, err := filepath.EvalSymlinks(hw); err == nil {
		return strings.TrimPrefix(p, root)
	}
	return strings.TrimPrefix(hw, root)
}

func stableFanID(chip, device string, idx int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", chip, device, idx)))
	return hex.EncodeToString(sum[:])[:16]
}

func (m *FanManager) Scan() (Snapshot, error) {
	pattern := filepath.Join(m.root, "class", "hwmon", "hwmon*")
	hwmons, err := filepath.Glob(pattern)
	if err != nil {
		return Snapshot{}, err
	}

	cfg := m.cfg.Snapshot()
	var snap Snapshot

	for _, hw := range hwmons {
		chip, _ := readText(filepath.Join(hw, "name"))
		if chip == "" {
			chip = filepath.Base(hw)
		}
		deviceID := stableDeviceIdentity(hw, m.root)

		entries, err := os.ReadDir(hw)
		if err != nil {
			continue
		}
		indices := map[int]bool{}
		for _, e := range entries {
			if mm := fanInputRE.FindStringSubmatch(e.Name()); mm != nil {
				i, _ := strconv.Atoi(mm[1])
				indices[i] = true
			}
			if mm := pwmRE.FindStringSubmatch(e.Name()); mm != nil {
				i, _ := strconv.Atoi(mm[1])
				indices[i] = true
			}
		}

		idxs := make([]int, 0, len(indices))
		for i := range indices {
			idxs = append(idxs, i)
		}
		sort.Ints(idxs)

		for _, idx := range idxs {
			id := stableFanID(chip, deviceID, idx)
			label, _ := readText(filepath.Join(hw, fmt.Sprintf("fan%d_label", idx)))
			if label == "" {
				label = fmt.Sprintf("%s Fan %d", chip, idx)
			}
			name := label
			if alias := strings.TrimSpace(cfg.Aliases[id]); alias != "" {
				name = alias
			}

			fan := Fan{ID: id, Index: idx, Name: name, DefaultName: label, Chip: chip, Device: deviceID}
			if v, err := readInt(filepath.Join(hw, fmt.Sprintf("fan%d_input", idx))); err == nil {
				fan.RPM = v
			}

			pwmPath := filepath.Join(hw, fmt.Sprintf("pwm%d", idx))
			if _, err := os.Stat(pwmPath); err == nil {
				fan.PWMPath = pwmPath
				fan.Writable = fileWritable(pwmPath)
				if v, err := readInt(pwmPath); err == nil {
					fan.PWM = v
					pct := int(math.Round(float64(*v) * 100 / 255))
					fan.Percent = &pct
				}
			}

			enablePath := filepath.Join(hw, fmt.Sprintf("pwm%d_enable", idx))
			if _, err := os.Stat(enablePath); err == nil {
				fan.PWMEnablePath = enablePath
				fan.ModeWritable = fileWritable(enablePath)
				if v, err := readInt(enablePath); err == nil {
					fan.PWMMode = v
				}
			}

			m.mu.Lock()
			_, fan.CanRestoreMode = m.originalModes[id]
			m.mu.Unlock()
			snap.Fans = append(snap.Fans, fan)
		}

		for _, e := range entries {
			mm := tempRE.FindStringSubmatch(e.Name())
			if mm == nil {
				continue
			}
			idx, _ := strconv.Atoi(mm[1])
			v, err := readInt(filepath.Join(hw, e.Name()))
			if err != nil {
				continue
			}
			label, _ := readText(filepath.Join(hw, fmt.Sprintf("temp%d_label", idx)))
			if label == "" {
				label = fmt.Sprintf("Temp %d", idx)
			}
			snap.Temperatures = append(snap.Temperatures, Temperature{
				Chip: chip, Label: label, Celsius: float64(*v) / 1000,
			})
		}
	}

	sort.Slice(snap.Fans, func(i, j int) bool {
		if snap.Fans[i].Chip == snap.Fans[j].Chip {
			return snap.Fans[i].Index < snap.Fans[j].Index
		}
		return snap.Fans[i].Chip < snap.Fans[j].Chip
	})
	return snap, nil
}

func (m *FanManager) findFan(id string) (Fan, error) {
	snap, err := m.Scan()
	if err != nil {
		return Fan{}, err
	}
	for _, f := range snap.Fans {
		if f.ID == id {
			return f, nil
		}
	}
	return Fan{}, errors.New("fan not found")
}

func writeSysfs(path string, value int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(value)), 0)
}

func (m *FanManager) SetPercent(id string, percent int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fan, err := m.findFanUnlocked(id)
	if err != nil {
		return err
	}
	if fan.PWMPath == "" || !fan.Writable {
		return errors.New("fan does not expose a writable PWM channel")
	}
	cfg := m.cfg.Snapshot()
	if percent < cfg.MinPercent {
		return fmt.Errorf("refusing %d%%: configured safety floor is %d%%", percent, cfg.MinPercent)
	}
	if percent > 100 {
		percent = 100
	}

	if fan.PWMEnablePath != "" && fan.ModeWritable {
		if _, ok := m.originalModes[id]; !ok && fan.PWMMode != nil {
			m.originalModes[id] = *fan.PWMMode
		}
		if fan.PWMMode == nil || *fan.PWMMode != 1 {
			if err := writeSysfs(fan.PWMEnablePath, 1); err != nil {
				return fmt.Errorf("enable manual PWM: %w", err)
			}
		}
	}

	pwm := int(math.Round(float64(percent) * 255 / 100))
	if err := writeSysfs(fan.PWMPath, pwm); err != nil {
		return fmt.Errorf("write PWM: %w", err)
	}
	return nil
}

// findFanUnlocked scans without consulting originalModes while m.mu is held.
func (m *FanManager) findFanUnlocked(id string) (Fan, error) {
	pattern := filepath.Join(m.root, "class", "hwmon", "hwmon*")
	hwmons, _ := filepath.Glob(pattern)
	cfg := m.cfg.Snapshot()
	for _, hw := range hwmons {
		chip, _ := readText(filepath.Join(hw, "name"))
		if chip == "" {
			chip = filepath.Base(hw)
		}
		deviceID := stableDeviceIdentity(hw, m.root)
		entries, _ := os.ReadDir(hw)
		indices := map[int]bool{}
		for _, e := range entries {
			if mm := fanInputRE.FindStringSubmatch(e.Name()); mm != nil {
				i, _ := strconv.Atoi(mm[1])
				indices[i] = true
			}
			if mm := pwmRE.FindStringSubmatch(e.Name()); mm != nil {
				i, _ := strconv.Atoi(mm[1])
				indices[i] = true
			}
		}
		for idx := range indices {
			fid := stableFanID(chip, deviceID, idx)
			if fid != id {
				continue
			}
			label, _ := readText(filepath.Join(hw, fmt.Sprintf("fan%d_label", idx)))
			if label == "" {
				label = fmt.Sprintf("%s Fan %d", chip, idx)
			}
			name := label
			if alias := strings.TrimSpace(cfg.Aliases[fid]); alias != "" {
				name = alias
			}
			fan := Fan{ID: fid, Index: idx, Name: name, DefaultName: label, Chip: chip, Device: deviceID}
			if v, err := readInt(filepath.Join(hw, fmt.Sprintf("fan%d_input", idx))); err == nil {
				fan.RPM = v
			}
			pwmPath := filepath.Join(hw, fmt.Sprintf("pwm%d", idx))
			if _, err := os.Stat(pwmPath); err == nil {
				fan.PWMPath = pwmPath
				fan.Writable = fileWritable(pwmPath)
				if v, err := readInt(pwmPath); err == nil {
					fan.PWM = v
					pct := int(math.Round(float64(*v) * 100 / 255))
					fan.Percent = &pct
				}
			}
			enablePath := filepath.Join(hw, fmt.Sprintf("pwm%d_enable", idx))
			if _, err := os.Stat(enablePath); err == nil {
				fan.PWMEnablePath = enablePath
				fan.ModeWritable = fileWritable(enablePath)
				if v, err := readInt(enablePath); err == nil {
					fan.PWMMode = v
				}
			}
			return fan, nil
		}
	}
	return Fan{}, errors.New("fan not found")
}

func (m *FanManager) Restore(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mode, ok := m.originalModes[id]
	if !ok {
		return errors.New("no startup PWM mode captured for this fan")
	}
	fan, err := m.findFanUnlocked(id)
	if err != nil {
		return err
	}
	if fan.PWMEnablePath == "" || !fan.ModeWritable {
		return errors.New("PWM mode is not writable")
	}
	if err := writeSysfs(fan.PWMEnablePath, mode); err != nil {
		return err
	}
	delete(m.originalModes, id)
	return nil
}

func (m *FanManager) ApplyProfile(name string) error {
	cfg := m.cfg.Snapshot()
	percent, ok := cfg.Profiles[name]
	if !ok {
		return errors.New("unknown profile")
	}
	snap, err := m.Scan()
	if err != nil {
		return err
	}
	var errs []string
	changed := 0
	for _, f := range snap.Fans {
		if !f.Writable {
			continue
		}
		if err := m.SetPercent(f.ID, percent); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f.Name, err))
			continue
		}
		changed++
	}
	if changed == 0 {
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		return errors.New("no writable PWM channels found")
	}
	if err := m.cfg.SetLastProfile(name); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("profile applied to %d fan(s), but some failed: %s", changed, strings.Join(errs, "; "))
	}
	return nil
}

func (m *FanManager) RestoreAll() error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.originalModes))
	for id := range m.originalModes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	if len(ids) == 0 {
		return errors.New("no captured startup PWM modes to restore")
	}
	var errs []string
	for _, id := range ids {
		if err := m.Restore(id); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
