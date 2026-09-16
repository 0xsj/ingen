package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"ingen/hammond/internal/governance"
)

var ErrMembershipVersionNotFound = errors.New("Hammond membership version not found")
var ErrMembershipConflict = errors.New("Hammond membership version conflict")

// MembershipVersionStore retains the latest accepted reference for each
// membership identity. It does not store provider bytes or verify signatures;
// callers must provide a snapshot that has already passed their trust checks.
type MembershipVersionStore interface {
	Accept(governance.MembershipSnapshot) error
	Current(string) (governance.MembershipReference, error)
}

// FileMembershipVersionStore is a local monotonic version ledger for verified
// membership snapshots. Its files contain references only, not snapshots.
type FileMembershipVersionStore struct {
	root string
	mu   sync.Mutex
}

type membershipVersionDocument struct {
	Schema           string              `json:"schema"`
	ID               string              `json:"id"`
	Version          int                 `json:"version"`
	MembershipSchema string              `json:"membership_schema"`
	Artifact         governance.Artifact `json:"artifact"`
}

const membershipVersionSchema = "ingen.hammond-membership-version/v1"

func NewFileMembershipVersionStore(root string) (*FileMembershipVersionStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("Hammond membership version store root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Hammond membership version store root: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create Hammond membership version store root: %w", err)
	}
	return &FileMembershipVersionStore{root: absoluteRoot}, nil
}

// Accept records a snapshot reference if it is newer than the current one.
// Equal references are idempotent; equal versions with different digests and
// lower versions are conflicts.
func (s *FileMembershipVersionStore) Accept(snapshot governance.MembershipSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("validate Hammond membership before persistence: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockFile(false)
	if err != nil {
		return err
	}
	defer unlock()

	path := s.pathFor(snapshot.Reference.ID)
	current, err := s.read(path)
	if err != nil && !errors.Is(err, ErrMembershipVersionNotFound) {
		return err
	}
	if err == nil {
		if snapshot.Reference.Version < current.Version {
			return fmt.Errorf("%w: received version %d after current version %d", ErrMembershipConflict, snapshot.Reference.Version, current.Version)
		}
		if snapshot.Reference.Version == current.Version {
			if snapshot.Reference.Equal(current) {
				return nil
			}
			return fmt.Errorf("%w: version %d has a different artifact", ErrMembershipConflict, snapshot.Reference.Version)
		}
	}
	return s.write(path, snapshot.Reference)
}

// Current returns the latest accepted reference for a membership identity.
func (s *FileMembershipVersionStore) Current(id string) (governance.MembershipReference, error) {
	if strings.TrimSpace(id) == "" {
		return governance.MembershipReference{}, fmt.Errorf("Hammond membership id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockFile(true)
	if err != nil {
		return governance.MembershipReference{}, err
	}
	defer unlock()
	return s.read(s.pathFor(id))
}

func (s *FileMembershipVersionStore) pathFor(id string) string {
	digest := sha256.Sum256([]byte(governance.MembershipSchema + "\x00" + id))
	return filepath.Join(s.root, hex.EncodeToString(digest[:])+".json")
}

func (s *FileMembershipVersionStore) read(path string) (governance.MembershipReference, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return governance.MembershipReference{}, fmt.Errorf("%w: %s", ErrMembershipVersionNotFound, path)
		}
		return governance.MembershipReference{}, fmt.Errorf("read Hammond membership version: %w", err)
	}
	var document membershipVersionDocument
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return governance.MembershipReference{}, fmt.Errorf("decode Hammond membership version: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return governance.MembershipReference{}, fmt.Errorf("decode Hammond membership version: multiple JSON values are not allowed")
		}
		return governance.MembershipReference{}, fmt.Errorf("decode Hammond membership version: trailing JSON is not allowed: %w", err)
	}
	if err := validateMembershipVersionDocument(document); err != nil {
		return governance.MembershipReference{}, err
	}
	return governance.MembershipReference{
		ID:       document.ID,
		Version:  document.Version,
		Schema:   document.MembershipSchema,
		Artifact: document.Artifact,
	}, nil
}

func validateMembershipVersionDocument(document membershipVersionDocument) error {
	if document.Schema != membershipVersionSchema {
		return fmt.Errorf("Hammond membership version schema must be %s", membershipVersionSchema)
	}
	if strings.TrimSpace(document.ID) == "" {
		return fmt.Errorf("Hammond membership version id is required")
	}
	if document.Version < 1 {
		return fmt.Errorf("Hammond membership version must be positive")
	}
	if document.MembershipSchema != governance.MembershipSchema {
		return fmt.Errorf("Hammond membership version membership schema must be %s", governance.MembershipSchema)
	}
	if strings.TrimSpace(document.Artifact.URI) == "" {
		return fmt.Errorf("Hammond membership version artifact URI is required")
	}
	if !validStoredSHA256(document.Artifact.SHA256) {
		return fmt.Errorf("Hammond membership version artifact sha256 must be a lowercase SHA-256 digest")
	}
	return nil
}

func (s *FileMembershipVersionStore) write(path string, reference governance.MembershipReference) error {
	data, err := json.MarshalIndent(membershipVersionDocument{
		Schema:           membershipVersionSchema,
		ID:               reference.ID,
		Version:          reference.Version,
		MembershipSchema: reference.Schema,
		Artifact:         reference.Artifact,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Hammond membership version: %w", err)
	}
	if err := writeBytes(path, append(data, '\n')); err != nil {
		return fmt.Errorf("publish Hammond membership version: %w", err)
	}
	return nil
}

func validStoredSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (s *FileMembershipVersionStore) lockFile(shared bool) (func(), error) {
	path := filepath.Join(s.root, ".hammond-membership.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Hammond membership version lock: %w", err)
	}
	operation := syscall.LOCK_EX
	if shared {
		operation = syscall.LOCK_SH
	}
	if err := syscall.Flock(int(file.Fd()), operation); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock Hammond membership version store: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
