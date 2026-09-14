package api

import (
	billing "example.com/paddock/modular-violating/internal/billing/domain"
	orders "example.com/paddock/modular-violating/internal/orders/domain"
)

func Create(id string) orders.Order {
	return orders.Order{ID: id}
}

func AttachInvoice(invoice billing.Invoice) string {
	return invoice.ID
}
