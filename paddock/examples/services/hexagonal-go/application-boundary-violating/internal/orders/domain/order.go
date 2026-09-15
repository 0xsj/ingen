package domain

type Order struct {
	ID string
}

func NewOrder(id string) (Order, error) {
	return Order{ID: id}, nil
}
