package application

import (
	"example.com/paddock/hexagonal-application-boundary-violating/internal/orders/adapters/outbound"
	"example.com/paddock/hexagonal-application-boundary-violating/internal/orders/domain"
	"example.com/paddock/hexagonal-application-boundary-violating/internal/orders/ports"
)

type Service struct {
	store        ports.Store
	defaultStore outbound.MemoryStore
}

func New(store ports.Store) Service {
	return Service{store: store, defaultStore: outbound.MemoryStore{}}
}

func (s Service) Create(id string) (domain.Order, error) {
	order, err := domain.NewOrder(id)
	if err != nil {
		return domain.Order{}, err
	}
	return order, s.store.Save(order)
}
