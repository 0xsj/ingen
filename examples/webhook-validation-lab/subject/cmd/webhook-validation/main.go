package main

import (
	"flag"
	"log"
	"net/http"

	webhookvalidation "ingen/examples/webhook-validation-lab/subject"
)

func main() {
	address := flag.String("addr", ":8090", "HTTP listen address")
	flag.Parse()

	log.Printf("webhook-validation subject listening on %s", *address)
	if err := http.ListenAndServe(*address, webhookvalidation.NewHandler(webhookvalidation.NewStore())); err != nil {
		log.Fatal(err)
	}
}
