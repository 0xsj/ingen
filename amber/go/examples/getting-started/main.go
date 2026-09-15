package main

import (
	"context"
	"fmt"

	amber "github.com/0xsj/ingen/amber"
)

func main() {
	root, err := amber.Start()
	if err != nil {
		panic(err)
	}

	ctx, err := amber.WithProvenance(context.Background(), root)
	if err != nil {
		panic(err)
	}

	incoming, ok := amber.ProvenanceFromContext(ctx)
	if !ok {
		panic("missing provenance")
	}

	child, err := incoming.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		panic(err)
	}

	fmt.Printf("root execution: %s\nchild execution: %s\n", root.ExecutionID(), child.ExecutionID())
}
