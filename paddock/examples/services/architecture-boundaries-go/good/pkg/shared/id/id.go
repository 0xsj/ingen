package id

type ID string

func New(value string) ID {
	return ID(value)
}
