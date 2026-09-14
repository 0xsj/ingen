package main

import (
	"example.com/paddock/layered-good/internal/repository"
	"example.com/paddock/layered-good/internal/service"
	"example.com/paddock/layered-good/internal/transport"
)

func main() {
	orders := service.New(repository.Store{})
	_ = transport.Handler(orders)
}
