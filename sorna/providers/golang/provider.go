// Package goprovider prepares Go source variants without changing the source
// tree that the campaign was given.
package goprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"ingen/sorna/internal/campaign"
	"ingen/sorna/internal/mutation"
)

// MutateFunc applies one already-reviewed mutation to a copied Go source
// tree. The callback is deliberately given the copy, never the source root.
// It returns semantic provenance for the edit; the provider fills in the
// resulting source-tree and binary identities.
type MutateFunc func(variantRoot string, spec mutation.Spec) (campaign.ProviderProvenance, error)

// Request describes a Go source preparation campaign. OutputDir retains the
// copied variant sources; BinaryDir retains only the runnable artifacts that
// the managed subject is allowed to read.
type Request struct {
	Plan         campaign.Plan
	PlanSHA256   string
	SourceRoot   string
	OutputDir    string
	BinaryDir    string
	BuildPackage string
	BinaryName   string
	ProviderID   string
	Capabilities []campaign.ProviderCapability
	SubjectArgs  []string
	Mutate       MutateFunc
}

// Variant records the durable paths and built binary identity for one plan
// entry. The campaign evidence later records the live executable identity.
type Variant struct {
	Sequence     int
	MutationID   string
	SourceDir    string
	BinaryPath   string
	BinarySHA256 string
}

// Result contains the provider manifest handed to the existing Sorna campaign
// executor and the preparation records useful to a human reviewer.
type Result struct {
	Provider    campaign.ProviderManifest
	Variants    []Variant
	Preparation campaign.PreparationSummary
}

// Build copies the source root once per mutation, applies the supplied Go
// mutator to the copy, and builds a fresh executable. It refuses to reuse
// either output directory so a failed preparation cannot be mistaken for a
// clean new attempt.
func Build(request Request) (Result, error) {
	if problems := campaign.Validate(request.Plan); len(problems) > 0 {
		return Result{}, fmt.Errorf("invalid Go provider plan: %s", strings.Join(problems, "; "))
	}
	if strings.TrimSpace(request.SourceRoot) == "" {
		return Result{}, fmt.Errorf("Go provider source root must not be empty")
	}
	if strings.TrimSpace(request.OutputDir) == "" || strings.TrimSpace(request.BinaryDir) == "" {
		return Result{}, fmt.Errorf("Go provider output and binary directories must not be empty")
	}
	if strings.TrimSpace(request.BuildPackage) == "" {
		return Result{}, fmt.Errorf("Go provider build package must not be empty")
	}
	if strings.TrimSpace(request.PlanSHA256) == "" {
		return Result{}, fmt.Errorf("Go provider plan hash must not be empty")
	}
	if strings.HasPrefix(request.BuildPackage, "-") {
		return Result{}, fmt.Errorf("Go provider build package must be a package path, not a flag")
	}
	if request.Mutate == nil {
		return Result{}, fmt.Errorf("Go provider mutator must not be nil")
	}
	if len(request.Capabilities) == 0 {
		return Result{}, fmt.Errorf("Go provider capabilities must not be empty")
	}
	declared := campaign.ProviderManifest{Capabilities: request.Capabilities}
	for _, planned := range request.Plan.Mutations {
		if !declared.Supports(planned.Spec) {
			return Result{}, fmt.Errorf("Go provider does not declare capability for %s", planned.Spec.ID)
		}
	}
	if request.BinaryName == "" {
		request.BinaryName = "subject"
	}
	if filepath.Base(request.BinaryName) != request.BinaryName || request.BinaryName == "." || request.BinaryName == ".." {
		return Result{}, fmt.Errorf("Go provider binary name must be a simple file name")
	}
	if request.ProviderID == "" {
		request.ProviderID = "golang-source-provider"
	}

	sourceRoot, err := absolutePath(request.SourceRoot)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Go provider source root: %w", err)
	}
	outputDir, err := rootedPath(sourceRoot, request.OutputDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Go provider output directory: %w", err)
	}
	binaryDir, err := rootedPath(sourceRoot, request.BinaryDir)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Go provider binary directory: %w", err)
	}
	if outputDir == sourceRoot || binaryDir == sourceRoot {
		return Result{}, fmt.Errorf("Go provider outputs must not be the source root")
	}
	if !pathWithin(sourceRoot, outputDir) || !pathWithin(sourceRoot, binaryDir) {
		return Result{}, fmt.Errorf("Go provider outputs must stay inside the source root")
	}
	if pathsOverlap(outputDir, binaryDir) {
		return Result{}, fmt.Errorf("Go provider source and binary outputs must be separate")
	}
	for name, path := range map[string]string{"output": outputDir, "binary": binaryDir} {
		if _, statErr := os.Stat(path); statErr == nil {
			return Result{}, fmt.Errorf("Go provider %s directory already exists: %s", name, path)
		} else if !os.IsNotExist(statErr) {
			return Result{}, fmt.Errorf("inspect Go provider %s directory: %w", name, statErr)
		}
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "go.mod")); err != nil {
		return Result{}, fmt.Errorf("Go provider source root must contain go.mod: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o755); err != nil {
		return Result{}, fmt.Errorf("create Go provider output parent: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(binaryDir), 0o755); err != nil {
		return Result{}, fmt.Errorf("create Go provider binary parent: %w", err)
	}
	stagedOutput, err := os.MkdirTemp(filepath.Dir(outputDir), filepath.Base(outputDir)+".staging-")
	if err != nil {
		return Result{}, fmt.Errorf("create Go provider output staging directory: %w", err)
	}
	stagedBinary, err := os.MkdirTemp(filepath.Dir(binaryDir), filepath.Base(binaryDir)+".staging-")
	if err != nil {
		_ = os.RemoveAll(stagedOutput)
		return Result{}, fmt.Errorf("create Go provider binary staging directory: %w", err)
	}
	keepOutput := false
	keepBinary := false
	defer func() {
		if !keepOutput {
			_ = os.RemoveAll(stagedOutput)
		}
		if !keepBinary {
			_ = os.RemoveAll(stagedBinary)
		}
	}()

	provider := campaign.ProviderManifest{
		Schema:       campaign.ProviderSchema,
		ID:           request.ProviderID,
		Version:      1,
		PlanSchema:   campaign.Schema,
		PlanSHA256:   request.PlanSHA256,
		Capabilities: append([]campaign.ProviderCapability(nil), request.Capabilities...),
		Entries:      make([]campaign.ProviderEntry, 0, len(request.Plan.Mutations)),
	}
	variants := make([]Variant, 0, len(request.Plan.Mutations))
	preparationVariants := make([]campaign.PreparationVariant, 0, len(request.Plan.Mutations))
	skips := []string{outputDir, binaryDir, stagedOutput, stagedBinary}
	for _, planned := range request.Plan.Mutations {
		name := fmt.Sprintf("%03d-%s", planned.Sequence, safeID(planned.Spec.ID))
		variantDir := filepath.Join(stagedOutput, "variants", name)
		sourceDir := filepath.Join(variantDir, "source")
		if err := copySourceTree(sourceRoot, sourceDir, []string{outputDir, binaryDir, stagedOutput, stagedBinary}); err != nil {
			return Result{}, fmt.Errorf("copy source for mutation %s: %w", planned.Spec.ID, err)
		}
		provenance, err := request.Mutate(sourceDir, planned.Spec)
		if err != nil {
			return Result{}, fmt.Errorf("apply mutation %s: %w", planned.Spec.ID, err)
		}
		sourceHash, err := campaign.HashTree(sourceDir)
		if err != nil {
			return Result{}, fmt.Errorf("hash mutated source for %s: %w", planned.Spec.ID, err)
		}
		changedFiles, err := changedFiles(sourceRoot, sourceDir, skips)
		if err != nil {
			return Result{}, fmt.Errorf("summarize source mutation %s: %w", planned.Spec.ID, err)
		}
		if len(changedFiles) == 0 {
			return Result{}, fmt.Errorf("mutation %s did not change any source files", planned.Spec.ID)
		}
		stagedBinaryPath := filepath.Join(stagedBinary, name, request.BinaryName)
		if err := os.MkdirAll(filepath.Dir(stagedBinaryPath), 0o755); err != nil {
			return Result{}, fmt.Errorf("create binary directory for mutation %s: %w", planned.Spec.ID, err)
		}
		if err := build(sourceDir, request.BuildPackage, stagedBinaryPath); err != nil {
			return Result{}, fmt.Errorf("build mutation %s: %w", planned.Spec.ID, err)
		}
		binaryPath := filepath.Join(binaryDir, name, request.BinaryName)
		command, err := relativeCommand(sourceRoot, binaryPath)
		if err != nil {
			return Result{}, fmt.Errorf("bind binary for mutation %s: %w", planned.Spec.ID, err)
		}
		finalSourceDir := filepath.Join(outputDir, "variants", name, "source")
		sourceRelative, err := filepath.Rel(sourceRoot, finalSourceDir)
		if err != nil {
			return Result{}, fmt.Errorf("bind source for mutation %s: %w", planned.Spec.ID, err)
		}
		binaryHash, err := hashFile(stagedBinaryPath)
		if err != nil {
			return Result{}, fmt.Errorf("hash binary for mutation %s: %w", planned.Spec.ID, err)
		}
		var targetResolution *campaign.TargetResolution
		if provenance.TargetResolution != nil {
			copy := *provenance.TargetResolution
			targetResolution = &copy
		}
		entryProvenance := &campaign.ProviderProvenance{
			SourceDir:        filepath.ToSlash(sourceRelative),
			SourceSHA256:     sourceHash,
			BinarySHA256:     binaryHash,
			Location:         provenance.Location,
			Before:           provenance.Before,
			After:            provenance.After,
			TargetResolution: targetResolution,
		}
		provider.Entries = append(provider.Entries, campaign.ProviderEntry{
			MutationID:  planned.Spec.ID,
			Command:     command,
			Args:        append([]string(nil), request.SubjectArgs...),
			SubjectRoot: ".",
			Variant:     planned.Spec.ID,
			Provenance:  entryProvenance,
		})
		variants = append(variants, Variant{
			Sequence:     planned.Sequence,
			MutationID:   planned.Spec.ID,
			SourceDir:    finalSourceDir,
			BinaryPath:   binaryPath,
			BinarySHA256: binaryHash,
		})
		preparationVariants = append(preparationVariants, campaign.PreparationVariant{
			Sequence:     planned.Sequence,
			MutationID:   planned.Spec.ID,
			SourceDir:    filepath.ToSlash(sourceRelative),
			SourceSHA256: sourceHash,
			BinaryPath:   command,
			BinarySHA256: binaryHash,
			ChangedFiles: changedFiles,
			Provenance:   cloneProvenance(entryProvenance),
		})
	}
	if problems := campaign.ValidateProvider(provider); len(problems) > 0 {
		return Result{}, fmt.Errorf("generated Go provider is invalid: %s", strings.Join(problems, "; "))
	}
	if err := os.Rename(stagedOutput, outputDir); err != nil {
		return Result{}, fmt.Errorf("publish Go provider source variants: %w", err)
	}
	keepOutput = true
	if err := os.Rename(stagedBinary, binaryDir); err != nil {
		_ = os.RemoveAll(outputDir)
		keepOutput = false
		return Result{}, fmt.Errorf("publish Go provider binaries: %w", err)
	}
	keepBinary = true
	return Result{
		Provider: provider,
		Variants: variants,
		Preparation: campaign.PreparationSummary{
			Schema:     campaign.PreparationSummarySchema,
			ProviderID: provider.ID,
			PlanPath:   "",
			PlanSHA256: provider.PlanSHA256,
			Variants:   preparationVariants,
		},
	}, nil
}

func cloneProvenance(provenance *campaign.ProviderProvenance) *campaign.ProviderProvenance {
	if provenance == nil {
		return nil
	}
	copy := *provenance
	if provenance.TargetResolution != nil {
		resolution := *provenance.TargetResolution
		copy.TargetResolution = &resolution
	}
	return &copy
}

func changedFiles(originalRoot, variantRoot string, skips []string) ([]string, error) {
	original, err := snapshotFiles(originalRoot, skips)
	if err != nil {
		return nil, fmt.Errorf("snapshot original source: %w", err)
	}
	variant, err := snapshotFiles(variantRoot, nil)
	if err != nil {
		return nil, fmt.Errorf("snapshot variant source: %w", err)
	}
	paths := make(map[string]bool)
	for path := range original {
		paths[path] = true
	}
	for path := range variant {
		paths[path] = true
	}
	changed := make([]string, 0)
	for path := range paths {
		if original[path] != variant[path] {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

func snapshotFiles(root string, skips []string) (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != root && isSkipped(path, skips) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path == root || entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported in source summary: %s", path)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported source summary entry %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest, err := hashFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = digest
		return nil
	})
	return files, err
}

// WriteManifest writes the provider handoff as a YAML document and returns
// the hash of the exact bytes written.
func WriteManifest(path string, provider campaign.ProviderManifest) (string, error) {
	if problems := campaign.ValidateProvider(provider); len(problems) > 0 {
		return "", fmt.Errorf("invalid Go provider manifest: %s", strings.Join(problems, "; "))
	}
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("Go provider manifest already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect Go provider manifest: %w", err)
	}
	document := struct {
		Provider campaign.ProviderManifest `yaml:"mutation_provider"`
	}{Provider: provider}
	contents, err := yaml.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode Go provider manifest: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return "", err
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:]), nil
}

func build(sourceDir, buildPackage, binaryPath string) error {
	command := exec.Command("go", "build", "-trimpath", "-o", binaryPath, buildPackage)
	command.Dir = sourceDir
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, trimmed)
	}
	return nil
}

func copySourceTree(sourceRoot, destination string, skips []string) error {
	return filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != sourceRoot && isSkipped(path, skips) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported in Go provider source: %s", path)
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported source entry %s", path)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, contents, info.Mode().Perm())
	})
}

func absolutePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func rootedPath(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Clean(filepath.Join(root, path)), nil
}

func relativeCommand(root, path string) (string, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("binary path must stay inside source root")
	}
	return filepath.ToSlash(relative), nil
}

func pathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func pathsOverlap(left, right string) bool {
	return left == right || pathWithin(left, right) || pathWithin(right, left)
}

func isSkipped(path string, skips []string) bool {
	for _, skip := range skips {
		if path == skip || pathWithin(skip, path) {
			return true
		}
	}
	base := filepath.Base(path)
	return base == ".git" || base == ".artifacts" || base == ".cache" || base == "node_modules"
}

func safeID(id string) string {
	var builder strings.Builder
	for _, character := range id {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('-')
		}
	}
	value := strings.Trim(builder.String(), "-.")
	if value == "" {
		return "mutation"
	}
	return value
}

func hashFile(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:]), nil
}
