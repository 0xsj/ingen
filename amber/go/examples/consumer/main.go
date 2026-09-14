package main

import (
	"fmt"
	"net/http"

	amber "github.com/0xsj/ingen/amber"
	amberhttp "github.com/0xsj/ingen/amber/adapters/http"
)

func main() {
	root, err := amber.Start()
	if err != nil {
		panic(err)
	}
	child, err := root.Child(amber.ChildOptions{Origin: amber.OriginIncoming})
	if err != nil {
		panic(err)
	}
	request, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
	if err != nil {
		panic(err)
	}
	outgoing, err := amberhttp.WithOutgoingRequest(request, child)
	if err != nil {
		panic(err)
	}
	fmt.Printf("consumer header present: %t\n", outgoing.Header.Get(amberhttp.HeaderName) != "")
}
