package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ingen/core/ciresult"
	"ingen/herdr-sentinel/evidence"
)

func roleVerifyCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel role verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", "", "absolute project root containing the role report")
	path := flags.String("path", "", "role execution report path relative to project root")
	expectedSHA := flags.String("expected-sha256", "", "expected raw role report SHA-256")
	ciResultPath := flags.String("ci-result", "", "optional CI result output path relative to project root")
	if err := flags.Parse(args); err != nil {
		return 1
	}
	if *root == "" || !filepath.IsAbs(*root) || *path == "" || len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "role verify requires an absolute --root and relative --path")
		return 1
	}
	if *expectedSHA != "" && !validEvidenceDigest(*expectedSHA) {
		fmt.Fprintln(os.Stderr, "--expected-sha256 must be a lowercase SHA-256 digest")
		return 1
	}
	verified, err := evidence.Verify(*root, *path, *expectedSHA)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *ciResultPath != "" {
		if err := rejectRoleCIResultOverlap(*ciResultPath, *path, verified); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		sourceRoot, rootErr := filepath.EvalSymlinks(*root)
		if rootErr != nil {
			fmt.Fprintln(os.Stderr, rootErr)
			return 1
		}
		result, resultErr := verified.BuildCIResult(sourceRoot)
		if resultErr != nil {
			fmt.Fprintln(os.Stderr, resultErr)
			return 1
		}
		if err := writeRoleCIResult(*root, *ciResultPath, result); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	code := roleEvidenceExitCode(verified.Report.Status, verified.Report.ExitCode)
	if verified.Report.ExitCode == nil {
		fmt.Printf("role execution %s: %s (contained command exit unavailable; report sha256 %s)\n", verified.Report.ExecutionID, verified.Report.Status, verified.ReportSHA256)
	} else {
		fmt.Printf("role execution %s: %s (contained command exit %d; report sha256 %s)\n", verified.Report.ExecutionID, verified.Report.Status, *verified.Report.ExitCode, verified.ReportSHA256)
	}
	return code
}

func rejectRoleCIResultOverlap(outputPath, reportPath string, verified evidence.Verified) error {
	output := filepath.Clean(outputPath)
	inputs := []string{filepath.Clean(reportPath), filepath.Clean(verified.Report.WorkspaceManifestPath), filepath.Clean(verified.Report.PolicyPath), filepath.Clean(verified.Report.StdoutPath), filepath.Clean(verified.Report.StderrPath)}
	for _, file := range verified.Files {
		inputs = append(inputs, filepath.Clean(file.Path))
	}
	if verified.Report.Governance != nil {
		for _, artifact := range verified.Report.Governance.ApprovalArtifacts {
			inputs = append(inputs, filepath.Clean(artifact.Path))
		}
	}
	for _, input := range inputs {
		if input == "." || input == "" || pathOverlaps(output, input) {
			return fmt.Errorf("CI result output %q overlaps verified role-execution evidence %q", outputPath, input)
		}
	}
	return nil
}

func pathOverlaps(left, right string) bool {
	left, right = filepath.ToSlash(filepath.Clean(left)), filepath.ToSlash(filepath.Clean(right))
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func roleEvidenceExitCode(status string, _ *int) int {
	switch status {
	case "completed":
		code, _ := ciresult.ExitCodeForStatus("passed")
		return code
	case "failed":
		code, _ := ciresult.ExitCodeForStatus("failed")
		return code
	case "canceled", "indeterminate":
		code, _ := ciresult.ExitCodeForStatus("error")
		return code
	}
	code, _ := ciresult.ExitCodeForStatus("error")
	return code
}

func roleExecuteErrorExitCode(exitCode *int) int {
	if exitCode != nil && *exitCode > 0 && *exitCode <= 255 {
		return *exitCode
	}
	return 1
}

func writeRoleCIResult(root, path string, artifact ciresult.Artifact) error {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(path) != path || strings.ContainsRune(path, '\\') || filepath.IsAbs(path) {
		return fmt.Errorf("CI result path must be a normalized project-relative path")
	}
	clean := filepath.Clean(path)
	if clean != path || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("CI result path %q is not normalized beneath the project root", path)
	}
	contents, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode role execution CI result: %w", err)
	}
	contents = append(contents, '\n')
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open project root for CI result: %w", err)
	}
	defer rooted.Close()
	if err := rejectRootedSymlinkComponents(rooted, clean); err != nil {
		return err
	}
	parent := filepath.ToSlash(filepath.Dir(clean))
	if parent != "." {
		if err := rooted.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create CI result directory: %w", err)
		}
	}
	if err := rejectRootedSymlinkComponents(rooted, clean); err != nil {
		return err
	}
	parent = filepath.ToSlash(filepath.Dir(clean))
	base := filepath.Base(clean)
	temp := filepath.ToSlash(filepath.Join(parent, "."+base+".tmp"))
	for suffix := 0; ; suffix++ {
		candidate := temp
		if suffix > 0 {
			candidate = temp + fmt.Sprintf(".%d", suffix)
		}
		file, err := rooted.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return fmt.Errorf("create temporary CI result: %w", err)
		}
		if _, err := file.Write(contents); err != nil {
			_ = file.Close()
			_ = rooted.Remove(candidate)
			return fmt.Errorf("write CI result: %w", err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			_ = rooted.Remove(candidate)
			return fmt.Errorf("sync CI result: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = rooted.Remove(candidate)
			return fmt.Errorf("close CI result: %w", err)
		}
		if err := rooted.Link(candidate, filepath.ToSlash(clean)); err != nil {
			_ = rooted.Remove(candidate)
			return fmt.Errorf("publish CI result without replacing existing files: %w", err)
		}
		if err := rooted.Remove(candidate); err != nil {
			return fmt.Errorf("remove temporary CI result link: %w", err)
		}
		directory, err := rooted.Open(parent)
		if err != nil {
			return fmt.Errorf("open CI result directory for sync: %w", err)
		}
		if err := directory.Sync(); err != nil {
			_ = directory.Close()
			return fmt.Errorf("sync CI result directory: %w", err)
		}
		if err := directory.Close(); err != nil {
			return fmt.Errorf("close CI result directory: %w", err)
		}
		return nil
	}
}

func rejectRootedSymlinkComponents(rooted *os.Root, relative string) error {
	current := ""
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "" || component == "." {
			continue
		}
		current = filepath.ToSlash(filepath.Join(current, component))
		info, err := rooted.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect CI result path component %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("CI result path component %q is a symbolic link", current)
		}
	}
	return nil
}

func validEvidenceDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
