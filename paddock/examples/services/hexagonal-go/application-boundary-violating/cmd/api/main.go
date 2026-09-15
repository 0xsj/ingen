package main

import (
	"example.com/paddock/hexagonal-application-boundary-violating/internal/orders/adapters/outbound"
	"example.com/paddock/hexagonal-application-boundary-violating/internal/orders/application"
)

func main() {
	_ = application.New(outbound.MemoryStore{})
}
