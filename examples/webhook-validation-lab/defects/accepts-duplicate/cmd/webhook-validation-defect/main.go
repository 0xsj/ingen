package main

import (
	"flag"
	"log"
	"net/http"

	acceptsduplicate "ingen/examples/webhook-validation-lab/defects/accepts-duplicate"
	webhookvalidation "ingen/examples/webhook-validation-lab/subject"
)

func main() {
	address := flag.String("addr", ":8091", "HTTP listen address")
	flag.Parse()

	subject := webhookvalidation.NewHandler(webhookvalidation.NewStore())
	log.Printf("accepts-duplicate defect subject listening on %s", *address)
	if err := http.ListenAndServe(*address, acceptsduplicate.NewHandler(subject)); err != nil {
		log.Fatal(err)
	}
}
