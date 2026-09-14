package domain

import "example.com/paddock/layered-violating/internal/transport"

type Order struct {
	ID string
}

func NewOrder(id string) Order {
	_ = transport.Boundary{}
	return Order{ID: id}
}
