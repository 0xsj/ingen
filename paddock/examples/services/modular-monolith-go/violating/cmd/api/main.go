package main

import (
	billing "example.com/paddock/modular-violating/internal/billing/api"
	orders "example.com/paddock/modular-violating/internal/orders/api"
)

func main() {
	_ = orders.Create("order-1")
	_ = billing.Create("invoice-1")
}
