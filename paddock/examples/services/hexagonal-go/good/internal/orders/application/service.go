package application

import (
	"example.com/paddock/hexagonal-good/internal/orders/domain"
	"example.com/paddock/hexagonal-good/internal/orders/ports"
)

type Service struct {
	store ports.Store
}

func New(store ports.Store) Service {
	return Service{store: store}
}

func (s Service) Create(id string) (domain.Order, error) {
	order, err := domain.NewOrder(id)
	if err != nil {
		return domain.Order{}, err
	}
	return order, s.store.Save(order)
}
