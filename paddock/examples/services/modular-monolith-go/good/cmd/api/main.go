package main

import (
	billing "example.com/paddock/modular-good/internal/billing/api"
	orders "example.com/paddock/modular-good/internal/orders/api"
)

func main() {
	_ = orders.Create("order-1")
	_ = billing.Create("invoice-1")
}
