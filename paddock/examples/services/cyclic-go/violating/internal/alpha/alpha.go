package alpha

import "example.com/paddock-cyclic/internal/beta"

func Name() string {
	return "alpha-" + beta.Name()
}

