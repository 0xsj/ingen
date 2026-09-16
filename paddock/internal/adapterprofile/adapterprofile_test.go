package adapterprofile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/paddock/internal/adapterprofile"
)

func TestLoadAndResolveProfile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "adapter.yaml")
	contents := `schema: paddock.adapter-profile/v1
name: example
executable: ./adapter.sh
args:
  - "{{profile_dir}}/helper.py"
  - --workspace
  - "{{root}}"
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := adapterprofile.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	executable, args, err := profile.Resolve(filepath.Join(directory, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	if executable != filepath.Join(directory, "adapter.sh") || len(args) != 3 || args[0] != filepath.Join(directory, "helper.py") || args[2] != filepath.Join(directory, "workspace") {
		t.Fatalf("unexpected resolved profile: executable=%q args=%#v", executable, args)
	}
	_, templateArgs, err := profile.ResolveTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if templateArgs[2] != "{{root}}" {
		t.Fatalf("template root was expanded too early: %#v", templateArgs)
	}
}

func TestLoadRejectsInvalidProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adapter.yaml")
	if err := os.WriteFile(path, []byte("schema: paddock.adapter-profile/v1\nexecutable: python3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := adapterprofile.Load(path)
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("profile error = %v", err)
	}
}

func TestLoadRejectsUnknownTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adapter.yaml")
	contents := `schema: paddock.adapter-profile/v1
name: example
executable: python3
args: ["{{root}}", "{{unknown}}"]
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := adapterprofile.Load(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported template") {
		t.Fatalf("profile template error = %v", err)
	}
}

func TestIsSHA256(t *testing.T) {
	if !adapterprofile.IsSHA256(strings.Repeat("a", 64)) {
		t.Fatal("valid lowercase SHA-256 was rejected")
	}
	for _, value := range []string{"", strings.Repeat("a", 63), strings.Repeat("A", 64), strings.Repeat("g", 64)} {
		if adapterprofile.IsSHA256(value) {
			t.Fatalf("invalid SHA-256 was accepted: %q", value)
		}
	}
}
