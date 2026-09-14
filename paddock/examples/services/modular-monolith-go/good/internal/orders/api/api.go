package api

import "example.com/paddock/modular-good/internal/orders/domain"

func Create(id string) domain.Order {
	return domain.Order{ID: id}
}
