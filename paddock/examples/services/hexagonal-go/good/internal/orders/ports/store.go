package ports

import "example.com/paddock/hexagonal-good/internal/orders/domain"

type Store interface {
	Save(domain.Order) error
}
