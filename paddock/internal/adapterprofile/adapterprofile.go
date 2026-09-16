package adapterprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const Schema = "paddock.adapter-profile/v1"

// Profile names an external adapter once so callers can reuse it across graph,
// policy, and CI commands. Arguments may use {{root}} and {{profile_dir}}.
type Profile struct {
	Schema     string   `yaml:"schema" json:"schema"`
	Name       string   `yaml:"name" json:"name"`
	Executable string   `yaml:"executable" json:"executable"`
	Args       []string `yaml:"args,omitempty" json:"args,omitempty"`
	path       string
}

func Load(path string) (Profile, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return Profile{}, fmt.Errorf("resolve adapter profile: %w", err)
	}
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return Profile{}, fmt.Errorf("read adapter profile: %w", err)
	}
	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return Profile{}, fmt.Errorf("parse adapter profile: %w", err)
	}
	profile.path = absolutePath
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (p Profile) Validate() error {
	if p.Schema != Schema {
		return fmt.Errorf("adapter profile schema must be %s, got %q", Schema, p.Schema)
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("adapter profile name is required")
	}
	if strings.TrimSpace(p.Executable) == "" {
		return fmt.Errorf("adapter profile executable is required")
	}
	for index, arg := range p.Args {
		if strings.TrimSpace(arg) == "" {
			return fmt.Errorf("adapter profile argument %d must not be empty", index+1)
		}
	}
	return nil
}

// Resolve returns an executable and arguments ready for one source root.
func (p Profile) Resolve(root string) (string, []string, error) {
	if err := p.Validate(); err != nil {
		return "", nil, err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("resolve adapter profile root: %w", err)
	}
	return p.executablePath(), expandArgs(p.Args, absoluteRoot, filepath.Dir(p.path)), nil
}

// ResolveTemplate is used by multi-root policy workflows. It expands the
// profile location while preserving {{root}} for each case's source root.
func (p Profile) ResolveTemplate() (string, []string, error) {
	if err := p.Validate(); err != nil {
		return "", nil, err
	}
	return p.executablePath(), expandArgs(p.Args, "{{root}}", filepath.Dir(p.path)), nil
}

func (p Profile) executablePath() string {
	if filepath.IsAbs(p.Executable) || !strings.ContainsAny(p.Executable, `/\\`) {
		return p.Executable
	}
	return filepath.Join(filepath.Dir(p.path), p.Executable)
}

func expandArgs(args []string, root, profileDir string) []string {
	expanded := make([]string, len(args))
	for index, arg := range args {
		expanded[index] = strings.ReplaceAll(arg, "{{root}}", root)
		expanded[index] = strings.ReplaceAll(expanded[index], "{{profile_dir}}", profileDir)
	}
	return expanded
}
