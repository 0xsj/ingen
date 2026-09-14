package transport

import "example.com/paddock/layered-good/internal/service"

func Handler(orders service.Orders) service.Orders {
	return orders
}
