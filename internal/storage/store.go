package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const SchemaVersion = 4

type Profile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	Color        string   `json:"color,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	Executable   string   `json:"executable,omitempty"`
	Workdir      string   `json:"workdir,omitempty"`
	Args         []string `json:"args,omitempty"`
	WorkspaceID  string   `json:"workspace_id,omitempty"`
	PaneID       string   `json:"pane_id,omitempty"`
	Favorite     bool     `json:"favorite,omitempty"`
}
type Instance struct {
	ProfileID   string `json:"profile_id,omitempty"`
	PaneID      string `json:"pane_id"`
	AgentName   string `json:"agent_name"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
}
type Config struct {
	Version   int        `json:"version"`
	Profiles  []Profile  `json:"profiles"`
	Instances []Instance `json:"instances,omitempty"`
	Favorites []string   `json:"favorites,omitempty"`
}

func Validate(c Config) error {
	if c.Version < 1 || c.Version > SchemaVersion {
		return fmt.Errorf("unsupported config schema %d", c.Version)
	}
	seen := map[string]bool{}
	panes := map[string]bool{}
	for _, p := range c.Profiles {
		if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Name) == "" {
			return errors.New("profile id and name required")
		}
		if seen[p.ID] {
			return fmt.Errorf("duplicate profile id %q", p.ID)
		}
		seen[p.ID] = true
		if len([]rune(p.Name)) > 120 {
			return fmt.Errorf("profile %q name exceeds 120 characters", p.ID)
		}
		if len([]rune(p.Instructions)) > 4000 {
			return fmt.Errorf("profile %q identity instructions exceed 4000 characters", p.ID)
		}
		if p.Icon != "" && (len([]rune(p.Icon)) > 8 || hasControl(p.Icon)) {
			return fmt.Errorf("profile %q icon must be up to 8 printable characters", p.ID)
		}
		if p.Color != "" && !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(p.Color) {
			return fmt.Errorf("profile %q color must use #RRGGBB", p.ID)
		}
		for _, value := range []string{p.ID, p.Name, p.Description, p.Instructions, p.Provider, p.Executable, p.Workdir, p.WorkspaceID, p.PaneID} {
			if hasControl(value) {
				return fmt.Errorf("profile %q contains control characters", p.ID)
			}
		}
		if p.PaneID != "" {
			if panes[p.PaneID] {
				return fmt.Errorf("duplicate profile pane %q", p.PaneID)
			}
			panes[p.PaneID] = true
		}
		for _, arg := range p.Args {
			if hasControl(arg) {
				return fmt.Errorf("profile %q argument contains control characters", p.ID)
			}
		}
	}
	seenInstances := map[string]bool{}
	for _, instance := range c.Instances {
		if strings.TrimSpace(instance.PaneID) == "" || strings.TrimSpace(instance.AgentName) == "" || strings.TrimSpace(instance.Name) == "" || hasControl(instance.PaneID) || hasControl(instance.AgentName) || hasControl(instance.Name) {
			return errors.New("agent instance pane and names are required")
		}
		if seenInstances[instance.PaneID] {
			return fmt.Errorf("duplicate agent instance pane %q", instance.PaneID)
		}
		seenInstances[instance.PaneID] = true
		for _, value := range []string{instance.ProfileID, instance.Description, instance.Provider, instance.Icon, instance.Color} {
			if hasControl(value) {
				return fmt.Errorf("agent instance %q contains control characters", instance.PaneID)
			}
		}
		if instance.Icon != "" && len([]rune(instance.Icon)) > 8 {
			return fmt.Errorf("agent instance %q icon exceeds 8 characters", instance.PaneID)
		}
		if instance.Color != "" && !regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`).MatchString(instance.Color) {
			return fmt.Errorf("agent instance %q color must use #RRGGBB", instance.PaneID)
		}
	}
	seenFavorites := map[string]bool{}
	for _, id := range c.Favorites {
		if strings.TrimSpace(id) == "" || hasControl(id) || seenFavorites[id] {
			return errors.New("favorite IDs must be non-empty and unique")
		}
		seenFavorites[id] = true
	}
	return nil
}
func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
func Load(dir string) (Config, error) {
	p := filepath.Join(dir, "agents.json")
	b, e := os.ReadFile(p)
	if errors.Is(e, os.ErrNotExist) {
		return Config{Version: SchemaVersion, Profiles: []Profile{}}, nil
	}
	if e != nil {
		return Config{}, e
	}
	var c Config
	if e = json.Unmarshal(b, &c); e != nil {
		return Config{}, fmt.Errorf("invalid config %s: %w", p, e)
	}
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Version == 1 {
		for _, profile := range c.Profiles {
			if profile.Favorite {
				id := profile.ID
				if profile.PaneID != "" {
					id = profile.PaneID
				}
				c.Favorites = append(c.Favorites, id)
			}
		}
		c.Version = SchemaVersion
	}
	if c.Version == 2 {
		c.Version = SchemaVersion
	}
	if c.Version == 3 {
		c.Version = SchemaVersion
	}
	if e = Validate(c); e != nil {
		return Config{}, e
	}
	return c, nil
}
func Save(dir string, c Config) error {
	if c.Version < SchemaVersion {
		c.Version = SchemaVersion
	}
	if e := Validate(c); e != nil {
		return e
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".agents-*.tmp")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(append(b, '\n')); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Chmod(name, 0600); e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(dir, "agents.json"))
}
