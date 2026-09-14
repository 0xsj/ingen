package api

import "example.com/paddock/modular-violating/internal/billing/domain"

func Create(id string) domain.Invoice {
	return domain.Invoice{ID: id}
}
