package domain

import (
	"example.com/paddock/architecture-boundaries-violating/internal/storage/blob"
	"example.com/paddock/architecture-boundaries-violating/pkg/shared/id"
)

type Order struct {
	ID   id.ID
	Blob blob.Info
}
