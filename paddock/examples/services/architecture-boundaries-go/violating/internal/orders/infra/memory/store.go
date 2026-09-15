package memory

import (
	"example.com/paddock/architecture-boundaries-violating/internal/orders/domain"
	"example.com/paddock/architecture-boundaries-violating/internal/orders/ports"
)

var _ ports.Store = Store{}

type Store struct{}

func (Store) Save(domain.Order) error {
	return nil
}
