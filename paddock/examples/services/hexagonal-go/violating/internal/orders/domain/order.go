package domain

import "net/http"

type Order struct {
	ID string
}

func NewOrder(id string) Order {
	_ = http.MethodGet
	return Order{ID: id}
}
