package domain

import "errors"

var ErrEmptyID = errors.New("empty order id")

type Order struct {
	ID string
}

func NewOrder(id string) (Order, error) {
	if id == "" {
		return Order{}, ErrEmptyID
	}
	return Order{ID: id}, nil
}
