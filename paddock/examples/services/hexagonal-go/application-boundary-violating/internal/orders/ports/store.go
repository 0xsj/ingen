package ports

import "example.com/paddock/hexagonal-application-boundary-violating/internal/orders/domain"

type Store interface {
	Save(domain.Order) error
}
