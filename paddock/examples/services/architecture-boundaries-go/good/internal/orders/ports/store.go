package ports

import "example.com/paddock/architecture-boundaries-good/internal/orders/domain"

type Store interface {
	Save(domain.Order) error
}
