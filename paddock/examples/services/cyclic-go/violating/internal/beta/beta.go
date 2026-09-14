package beta

import "example.com/paddock-cyclic/internal/alpha"

func Name() string {
	return "beta-" + alpha.Name()
}

