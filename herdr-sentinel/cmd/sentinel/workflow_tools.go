package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// workflowTools selects explicitly supplied installed tools or the existing
// development checkout. Installed tools never fall back to PATH or go run.
type workflowTools struct {
	dir       string
	installed bool
}

func selectWorkflowTools(flags *flag.FlagSet, checkout, binDir string, required ...string) (workflowTools, error) {
	if binDir == "" {
		if flagWasSet(flags, "tool-dir") {
			return workflowTools{}, fmt.Errorf("--tool-dir must name an absolute directory")
		}
		dir, err := absoluteDirectory(checkout, "InGen")
		return workflowTools{dir: dir}, err
	}
	if flagWasSet(flags, "ingen-root") || flagWasSet(flags, "malcolm-manifest") {
		return workflowTools{}, fmt.Errorf("--tool-dir cannot be combined with --ingen-root or --malcolm-manifest")
	}
	if !filepath.IsAbs(binDir) {
		return workflowTools{}, fmt.Errorf("--tool-dir must name an absolute directory")
	}
	dir, err := filepath.EvalSymlinks(binDir)
	if err != nil {
		return workflowTools{}, fmt.Errorf("resolve installed tool directory: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return workflowTools{}, fmt.Errorf("installed tool directory is unavailable")
	}
	for _, name := range required {
		info, err := os.Lstat(filepath.Join(dir, name))
		if err != nil {
			return workflowTools{}, fmt.Errorf("installed tool %s is unavailable: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return workflowTools{}, fmt.Errorf("installed tool %s must be a regular executable file", name)
		}
	}
	return workflowTools{dir: dir, installed: true}, nil
}

func (tools workflowTools) sorna(args ...string) *exec.Cmd {
	if tools.installed {
		return exec.Command(filepath.Join(tools.dir, "sorna"), args...)
	}
	command := exec.Command("go", append([]string{"run", "./sorna/cmd/sorna"}, args...)...)
	command.Dir = tools.dir
	return command
}
