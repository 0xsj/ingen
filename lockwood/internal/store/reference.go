package store

import (
	"fmt"
	"os"

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
