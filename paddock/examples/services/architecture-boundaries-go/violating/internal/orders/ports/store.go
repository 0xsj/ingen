package ports

import "example.com/paddock/architecture-boundaries-violating/internal/orders/domain"

type Store interface {
	Save(domain.Order) error
}
