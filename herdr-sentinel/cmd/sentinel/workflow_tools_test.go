package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledWorkflowSelectionFailsBeforeWritingOutputs(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "spec.malc"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "malcolm"), []byte("fixture executable"), 0700); err != nil {
		t.Fatal(err)
	}
	code := createContractCommand([]string{"--root", root, "--spec", "spec.malc", "--tool-dir", bin})
	if code == 0 {
		t.Fatal("accepted installed directory without the lowering bridge")
	}
	if _, err := os.Stat(filepath.Join(root, ".ingen")); !os.IsNotExist(err) {
		t.Fatalf("missing installed tools caused output creation: %v", err)
	}
}

func TestInstalledWorkflowRejectsAmbiguousOrNonExecutableTools(t *testing.T) {
	bin := t.TempDir()
	bin, err := filepath.EvalSymlinks(bin)
	if err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(bin, "sorna")
	if err := os.WriteFile(valid, []byte("fixture executable"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
		bin  string
	}{
		{"checkout-and-installed", []string{"--ingen-root", ".", "--tool-dir", bin}, bin},
		{"manifest-and-installed", []string{"--malcolm-manifest", "custom/Cargo.toml", "--tool-dir", bin}, bin},
		{"relative", []string{"--tool-dir", "bin"}, "bin"},
		{"explicit-empty", []string{"--tool-dir", ""}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			flags.String("ingen-root", ".", "")
			flags.String("tool-dir", "", "")
			flags.String("malcolm-manifest", "malcolm/Cargo.toml", "")
			if err := flags.Parse(test.args); err != nil {
				t.Fatal(err)
			}
			if _, err := selectWorkflowTools(flags, ".", test.bin, "sorna"); err == nil {
				t.Fatal("accepted invalid installed tool selection")
			}
		})
	}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	tools, err := selectWorkflowTools(flags, "/missing-checkout", bin, "sorna")
	if err != nil {
		t.Fatal(err)
	}
	cmd := tools.sorna("evidence", "verify", "fixture")
	if cmd.Path != valid || len(cmd.Args) != 4 || cmd.Dir != "" {
		t.Fatalf("installed command selected a checkout or compiler: %+v", cmd)
	}
	if err := os.Chmod(valid, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := selectWorkflowTools(flags, ".", bin, "sorna"); err == nil {
		t.Fatal("accepted nonexecutable installed tool")
	}
	if err := os.Remove(valid); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(os.Args[0], valid); err != nil {
		t.Fatal(err)
	}
	if _, err := selectWorkflowTools(flags, ".", bin, "sorna"); err == nil {
		t.Fatal("accepted installed binary symlink")
	}
}
