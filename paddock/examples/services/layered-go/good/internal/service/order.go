package service

import (
	"example.com/paddock/layered-good/internal/domain"
	"example.com/paddock/layered-good/internal/repository"
)

type Orders struct {
	store repository.Store
}

func New(store repository.Store) Orders {
	return Orders{store: store}
}

func (s Orders) Create(id string) domain.Order {
	order := domain.NewOrder(id)
	s.store.Save(order)
	return order
}
