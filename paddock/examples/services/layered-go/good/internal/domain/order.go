package domain

type Order struct {
	ID string
}

func NewOrder(id string) Order {
	return Order{ID: id}
}
