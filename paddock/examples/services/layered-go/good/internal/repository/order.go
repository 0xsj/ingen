package repository

import "example.com/paddock/layered-good/internal/domain"

type Store struct{}

func (Store) Save(order domain.Order) {}
