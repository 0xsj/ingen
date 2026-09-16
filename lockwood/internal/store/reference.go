package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ingen/lockwood/internal/artifact"
)

func (s *Filesystem) VerifyReference(reference artifact.Reference) error {
	if reference.Schema != artifact.Schema {
		return fmt.Errorf("unexpected artifact schema %q", reference.Schema)
	}
	path, err := s.blobPath(reference.Digest)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat artifact: %w", err)
	}
	if info.Size() != reference.SizeBytes {
		return fmt.Errorf("artifact size mismatch: got %d, want %d", info.Size(), reference.SizeBytes)
	}
	if err := s.Verify(reference.Digest); err != nil {
		return err
	}
	return nil
}

func validateReference(reference artifact.Reference) error {
	if reference.Schema != artifact.Schema {
		return fmt.Errorf("unexpected artifact schema %q", reference.Schema)
	}
	if err := artifact.ValidateDigest(reference.Digest); err != nil {
		return err
	}
	if reference.SizeBytes < 0 {
		return fmt.Errorf("artifact size cannot be negative")
	}
	if strings.TrimSpace(reference.MediaType) == "" {
		return fmt.Errorf("artifact media type is required")
	}
	return nil
}

func marshalReference(reference artifact.Reference) ([]byte, error) {
	if err := validateReference(reference); err != nil {
		return nil, err
	}
	return json.Marshal(reference)
}

func unmarshalReference(data []byte) (artifact.Reference, error) {
	var reference artifact.Reference
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&reference); err != nil {
		return artifact.Reference{}, fmt.Errorf("decode artifact reference: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return artifact.Reference{}, fmt.Errorf("artifact reference contains multiple JSON values")
		}
		return artifact.Reference{}, fmt.Errorf("decode artifact reference: %w", err)
	}
	if err := validateReference(reference); err != nil {
		return artifact.Reference{}, err
	}
	canonical, err := marshalReference(reference)
	if err != nil {
		return artifact.Reference{}, err
	}
	if !bytes.Equal(data, canonical) {
		return artifact.Reference{}, fmt.Errorf("artifact reference is not canonical JSON")
	}
	return reference, nil
}

func referenceKey(reference artifact.Reference) (string, error) {
	encoded, err := marshalReference(reference)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(encoded), nil
}

func (s *Filesystem) publishReference(reference artifact.Reference) error {
	encoded, err := marshalReference(reference)
	if err != nil {
		return fmt.Errorf("encode artifact reference: %w", err)
	}
	path, err := s.referencePath(reference, encoded)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create reference directory: %w", err)
	}
	if existing, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(existing, encoded) {
			return fmt.Errorf("artifact reference metadata conflict for %s", reference.Digest)
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return fmt.Errorf("sync reference directory: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect existing artifact reference: %w", err)
	}

	temp, err := os.CreateTemp(s.tempRoot, "reference-")
	if err != nil {
		return fmt.Errorf("create temporary artifact reference: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary artifact reference: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary artifact reference: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary artifact reference: %w", err)
	}
	if err := os.Link(tempName, path); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("publish artifact reference: %w", err)
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read racing artifact reference: %w", readErr)
		}
		if !bytes.Equal(existing, encoded) {
			return fmt.Errorf("artifact reference metadata conflict for %s", reference.Digest)
		}
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync reference directory: %w", err)
	}
	return nil
}

func (s *Filesystem) referencePath(reference artifact.Reference, encoded []byte) (string, error) {
	if err := validateReference(reference); err != nil {
		return "", err
	}
	hexDigest := strings.TrimPrefix(reference.Digest, artifact.SHA256Algorithm+":")
	metadataDigest := strings.TrimPrefix(artifact.DigestBytes(encoded), artifact.SHA256Algorithm+":")
	return filepath.Join(s.root, "references", artifact.SHA256Algorithm, hexDigest[:2], hexDigest[2:4], metadataDigest+".json"), nil
}

// ListReferences inventories persisted descriptive references. A reference
// may appear more than once for one blob because media type and logical name
// are contextual metadata rather than content identity.
func (s *Filesystem) ListReferences() ([]artifact.Reference, error) {
	root := filepath.Join(s.root, "references", artifact.SHA256Algorithm)
	firstLevel, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("list artifact references: %w", err)
	}
	references := make([]artifact.Reference, 0)
	for _, first := range firstLevel {
		if !first.IsDir() || !isLowerHexComponent(first.Name()) {
			return nil, fmt.Errorf("unexpected artifact reference directory: %s", first.Name())
		}
		secondLevel, err := os.ReadDir(filepath.Join(root, first.Name()))
		if err != nil {
			return nil, fmt.Errorf("list artifact reference partition %s: %w", first.Name(), err)
		}
		for _, second := range secondLevel {
			if !second.IsDir() || !isLowerHexComponent(second.Name()) {
				return nil, fmt.Errorf("unexpected artifact reference partition: %s/%s", first.Name(), second.Name())
			}
			files, err := os.ReadDir(filepath.Join(root, first.Name(), second.Name()))
			if err != nil {
				return nil, fmt.Errorf("list artifact reference files %s/%s: %w", first.Name(), second.Name(), err)
			}
			for _, file := range files {
				if file.IsDir() || file.Type()&os.ModeSymlink != 0 {
					return nil, fmt.Errorf("unexpected artifact reference entry: %s/%s/%s", first.Name(), second.Name(), file.Name())
				}
				metadataHex := strings.TrimSuffix(file.Name(), ".json")
				if !strings.HasSuffix(file.Name(), ".json") || !isLowerHexString(metadataHex) {
					return nil, fmt.Errorf("unexpected artifact reference name: %s/%s/%s", first.Name(), second.Name(), file.Name())
				}
				path := filepath.Join(root, first.Name(), second.Name(), file.Name())
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, fmt.Errorf("read artifact reference %s: %w", path, err)
				}
				reference, err := unmarshalReference(data)
				if err != nil {
					return nil, fmt.Errorf("validate artifact reference %s: %w", path, err)
				}
				hexDigest := strings.TrimPrefix(reference.Digest, artifact.SHA256Algorithm+":")
				metadataDigest, err := referenceKey(reference)
				if err != nil {
					return nil, fmt.Errorf("derive artifact reference metadata digest %s: %w", path, err)
				}
				if hexDigest[:2] != first.Name() || hexDigest[2:4] != second.Name() || strings.TrimPrefix(metadataDigest, artifact.SHA256Algorithm+":") != metadataHex {
					return nil, fmt.Errorf("artifact reference path does not match metadata: %s", path)
				}
				references = append(references, reference)
			}
		}
	}
	sort.Slice(references, func(i, j int) bool {
		if references[i].Digest != references[j].Digest {
			return references[i].Digest < references[j].Digest
		}
		if references[i].MediaType != references[j].MediaType {
			return references[i].MediaType < references[j].MediaType
		}
		return references[i].LogicalName < references[j].LogicalName
	})
	return references, nil
}

func isLowerHexString(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
