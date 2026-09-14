package application

import (
	"example.com/paddock/hexagonal-violating/internal/orders/domain"
	"example.com/paddock/hexagonal-violating/internal/orders/ports"
)

type Service struct {
	store ports.Store
}

func New(store ports.Store) Service {
	return Service{store: store}
}

func (s Service) Create(id string) (domain.Order, error) {
	order := domain.NewOrder(id)
	return order, s.store.Save(order)
}
