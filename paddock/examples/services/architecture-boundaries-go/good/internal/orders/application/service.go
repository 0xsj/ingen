package application

import (
	"example.com/paddock/architecture-boundaries-good/internal/orders/domain"
	"example.com/paddock/architecture-boundaries-good/internal/orders/ports"
)

type Service struct {
	store ports.Store
}

func New(store ports.Store) Service {
	return Service{store: store}
}

func (s Service) Create(value string) error {
	return s.store.Save(domain.NewOrder(value))
}
