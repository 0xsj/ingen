package ports

import "example.com/paddock/hexagonal-violating/internal/orders/domain"

type Store interface {
	Save(domain.Order) error
}
