package sorna

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/integrity"
)

const (
	EvidenceSchema       = "sorna.evidence/v1"
	OracleEvidenceSchema = "sorna.oracle-evidence/v1"
	EvidenceMediaType    = "application/vnd.sorna.evidence+tar"
	OracleMediaType      = "application/vnd.sorna.oracle-evidence+tar"
)

type ImportRequest struct {
	CustodyID      string
	ExpectedDigest string
	LogicalName    string
	MaxBytes       int64
	ReceivedAt     time.Time
	RetentionClass string
	Redaction      string
}

type Importer struct {
	ingestor *custody.Ingestor
}

func NewImporter(ingestor *custody.Ingestor) (*Importer, error) {
	if ingestor == nil {
		return nil, fmt.Errorf("custody ingestor is required")
	}
	return &Importer{ingestor: ingestor}, nil
}

func (i *Importer) Import(directory string, request ImportRequest) (custody.Record, error) {
	snapshot, err := readSnapshotWithLimit(directory, request.MaxBytes)
	if err != nil {
		return custody.Record{}, err
	}
	archive, err := deterministicTarWithLimit(snapshot.Files, request.MaxBytes)
	if err != nil {
		return custody.Record{}, fmt.Errorf("create deterministic Sorna archive: %w", err)
	}
	if request.LogicalName == "" {
		request.LogicalName = filepath.Base(filepath.Clean(directory)) + ".tar"
	}
	if request.RetentionClass == "" {
		request.RetentionClass = "default"
	}
	if request.Redaction == "" {
		request.Redaction = "none"
	}
	record, err := i.ingestor.Accept(bytes.NewReader(archive), custody.IntakeRequest{
		CustodyID:      request.CustodyID,
		ExpectedDigest: request.ExpectedDigest,
		MediaType:      snapshot.MediaType,
		LogicalName:    request.LogicalName,
		MaxBytes:       request.MaxBytes,
		ReceivedAt:     request.ReceivedAt,
		Producer:       custody.Producer{Tool: "sorna", Kind: snapshot.Kind},
		Source:         custody.Source{RunID: snapshot.RunID, Path: filepath.Clean(directory)},
		Handling:       custody.Handling{Redaction: request.Redaction, RetentionClass: request.RetentionClass},
	})
	if err != nil {
		return custody.Record{}, fmt.Errorf("ingest Sorna bundle: %w", err)
	}
	return record, nil
}

type snapshot struct {
	Files     []snapshotFile
	MediaType string
	Kind      string
	RunID     string
}

type snapshotFile struct {
	Name string
	Data []byte
}

func readSnapshot(directory string) (snapshot, error) {
	return readSnapshotWithLimit(directory, 0)
}

func readSnapshotWithLimit(directory string, maxBytes int64) (snapshot, error) {
	if strings.TrimSpace(directory) == "" {
		return snapshot{}, fmt.Errorf("Sorna bundle directory is required")
	}
	if maxBytes < 0 {
		return snapshot{}, fmt.Errorf("maximum artifact size cannot be negative")
	}
	rootInfo, err := os.Stat(directory)
	if err != nil {
		return snapshot{}, fmt.Errorf("inspect Sorna bundle: %w", err)
	}
	if !rootInfo.IsDir() {
		return snapshot{}, fmt.Errorf("Sorna bundle path is not a directory")
	}

	checksumPath := filepath.Join(directory, "checksums.sha256")
	checksumInfo, err := os.Lstat(checksumPath)
	if err != nil {
		return snapshot{}, fmt.Errorf("inspect Sorna checksums: %w", err)
	}
	if !checksumInfo.Mode().IsRegular() {
		return snapshot{}, fmt.Errorf("Sorna checksums path is not a regular file")
	}
	if maxBytes > 0 && checksumInfo.Size() > maxBytes {
		return snapshot{}, fmt.Errorf("Sorna bundle input exceeds maximum size of %d bytes", maxBytes)
	}
	checksumBytes, err := os.ReadFile(checksumPath)
	if err != nil {
		return snapshot{}, fmt.Errorf("read Sorna checksums: %w", err)
	}
	checksums, err := parseChecksums(string(checksumBytes))
	if err != nil {
		return snapshot{}, err
	}
	totalBytes := int64(len(checksumBytes))
	files := make(map[string][]byte, len(checksums)+1)
	for name, expected := range checksums {
		filePath, err := safeBundlePath(directory, name)
		if err != nil {
			return snapshot{}, err
		}
		info, err := os.Lstat(filePath)
		if err != nil {
			return snapshot{}, fmt.Errorf("inspect checksummed file %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			return snapshot{}, fmt.Errorf("checksummed path %s is not a regular file", name)
		}
		if maxBytes > 0 && (info.Size() > maxBytes-totalBytes || totalBytes > maxBytes) {
			return snapshot{}, fmt.Errorf("Sorna bundle input exceeds maximum size of %d bytes", maxBytes)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return snapshot{}, fmt.Errorf("read checksummed file %s: %w", name, err)
		}
		if err := integrity.VerifyBytes(data, artifact.SHA256Algorithm+":"+expected); err != nil {
			return snapshot{}, fmt.Errorf("checksum mismatch for %s", name)
		}
		files[name] = data
		totalBytes += int64(len(data))
	}
	manifest, ok := files["manifest.json"]
	if !ok {
		return snapshot{}, fmt.Errorf("Sorna checksums do not include manifest.json")
	}
	var envelope struct {
		Schema string `json:"schema"`
		RunID  string `json:"run_id"`
	}
	if err := json.Unmarshal(manifest, &envelope); err != nil {
		return snapshot{}, fmt.Errorf("decode Sorna manifest: %w", err)
	}
	result := snapshot{Files: make([]snapshotFile, 0, len(files)+1)}
	switch envelope.Schema {
	case EvidenceSchema:
		if strings.TrimSpace(envelope.RunID) == "" {
			return snapshot{}, fmt.Errorf("Sorna evidence manifest run_id is required")
		}
		result.MediaType = EvidenceMediaType
		result.Kind = "evidence-bundle"
		result.RunID = envelope.RunID
	case OracleEvidenceSchema:
		result.MediaType = OracleMediaType
		result.Kind = "oracle-evidence-bundle"
	default:
		return snapshot{}, fmt.Errorf("unsupported Sorna manifest schema %q", envelope.Schema)
	}

	seenFiles := make(map[string]bool, len(files))
	err = filepath.WalkDir(directory, func(currentPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if currentPath == directory {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Sorna bundle contains symlink %s", currentPath)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(directory, currentPath)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == "checksums.sha256" {
			seenFiles[name] = true
			return nil
		}
		if _, ok := checksums[name]; !ok {
			return fmt.Errorf("Sorna bundle contains unchecksummed file %s", name)
		}
		seenFiles[name] = true
		return nil
	})
	if err != nil {
		return snapshot{}, err
	}
	for name := range checksums {
		if !seenFiles[name] {
			return snapshot{}, fmt.Errorf("checksummed file %s was not found in bundle", name)
		}
	}
	for name, data := range files {
		result.Files = append(result.Files, snapshotFile{Name: name, Data: data})
	}
	result.Files = append(result.Files, snapshotFile{Name: "checksums.sha256", Data: checksumBytes})
	sort.Slice(result.Files, func(a, b int) bool { return result.Files[a].Name < result.Files[b].Name })
	return result, nil
}

func parseChecksums(contents string) (map[string]string, error) {
	checksums := make(map[string]string)
	for lineNumber, line := range strings.Split(strings.TrimSpace(contents), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != sha256.Size*2 || parts[1] == "" {
			return nil, fmt.Errorf("invalid Sorna checksum line %d", lineNumber+1)
		}
		if _, err := hex.DecodeString(parts[0]); err != nil || strings.ToLower(parts[0]) != parts[0] {
			return nil, fmt.Errorf("invalid Sorna checksum line %d", lineNumber+1)
		}
		name := path.Clean(strings.TrimSpace(parts[1]))
		if err := validateBundleName(name); err != nil {
			return nil, fmt.Errorf("checksum line %d: %w", lineNumber+1, err)
		}
		if name == "checksums.sha256" {
			return nil, fmt.Errorf("checksum list must not include checksums.sha256")
		}
		if _, exists := checksums[name]; exists {
			return nil, fmt.Errorf("duplicate Sorna checksum path %q", name)
		}
		checksums[name] = parts[0]
	}
	if len(checksums) == 0 {
		return nil, fmt.Errorf("Sorna checksum file contains no artifacts")
	}
	return checksums, nil
}

func validateBundleName(name string) error {
	if name == "." || name == "" || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") {
		return fmt.Errorf("unsafe bundle path %q", name)
	}
	return nil
}

func safeBundlePath(root, name string) (string, error) {
	if err := validateBundleName(name); err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(name)), nil
}

func deterministicTar(files []snapshotFile) ([]byte, error) {
	return deterministicTarWithLimit(files, 0)
}

func deterministicTarWithLimit(files []snapshotFile, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, fmt.Errorf("maximum artifact size cannot be negative")
	}
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, file := range files {
		header := &tar.Header{
			Name:     file.Name,
			Mode:     0o600,
			Size:     int64(len(file.Data)),
			ModTime:  time.Unix(0, 0).UTC(),
			Typeflag: tar.TypeReg,
		}
		if err := writer.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := writer.Write(file.Data); err != nil {
			return nil, err
		}
		if maxBytes > 0 && int64(buffer.Len()) > maxBytes {
			return nil, fmt.Errorf("Sorna archive exceeds maximum size of %d bytes", maxBytes)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(buffer.Len()) > maxBytes {
		return nil, fmt.Errorf("Sorna archive exceeds maximum size of %d bytes", maxBytes)
	}
	return buffer.Bytes(), nil
}
