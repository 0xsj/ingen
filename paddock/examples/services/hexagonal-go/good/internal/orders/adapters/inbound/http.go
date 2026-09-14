package inbound

import "example.com/paddock/hexagonal-good/internal/orders/application"

func Handler(service application.Service) application.Service {
	return service
}
