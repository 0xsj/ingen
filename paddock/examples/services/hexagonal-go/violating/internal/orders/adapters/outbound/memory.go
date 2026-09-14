package outbound

import "example.com/paddock/hexagonal-violating/internal/orders/domain"

type MemoryStore struct{}

func (MemoryStore) Save(domain.Order) error {
	return nil
}
