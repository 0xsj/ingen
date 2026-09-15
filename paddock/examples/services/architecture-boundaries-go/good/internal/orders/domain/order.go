package domain

import "example.com/paddock/architecture-boundaries-good/pkg/shared/id"

type Order struct {
	ID id.ID
}

func NewOrder(value string) Order {
	return Order{ID: id.New(value)}
}
