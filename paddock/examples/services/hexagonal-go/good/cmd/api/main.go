package main

import (
	"example.com/paddock/hexagonal-good/internal/orders/adapters/inbound"
	"example.com/paddock/hexagonal-good/internal/orders/adapters/outbound"
	"example.com/paddock/hexagonal-good/internal/orders/application"
)

func main() {
	service := application.New(outbound.MemoryStore{})
	_ = inbound.Handler(service)
}
