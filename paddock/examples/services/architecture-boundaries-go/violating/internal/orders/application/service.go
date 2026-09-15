package application

import (
	"example.com/paddock/architecture-boundaries-violating/internal/orders/domain"
	"example.com/paddock/architecture-boundaries-violating/internal/orders/infra/memory"
	"example.com/paddock/architecture-boundaries-violating/internal/orders/ports"
)

type Service struct {
	store ports.Store
}

func New(store memory.Store) Service {
	return Service{store: store}
}

func (s Service) Create(order domain.Order) error {
	return s.store.Save(order)
}
