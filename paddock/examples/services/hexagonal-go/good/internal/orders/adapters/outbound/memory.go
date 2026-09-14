package outbound

import "example.com/paddock/hexagonal-good/internal/orders/domain"

type MemoryStore struct{}

func (MemoryStore) Save(domain.Order) error {
	return nil
}
