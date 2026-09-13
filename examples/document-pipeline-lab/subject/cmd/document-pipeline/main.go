package main

import (
	"flag"
	"log"
	"net/http"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func main() {
	address := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	log.Printf("document-pipeline subject listening on %s", *address)
	if err := http.ListenAndServe(*address, documentpipeline.NewHandler(documentpipeline.NewStore())); err != nil {
		log.Fatal(err)
	}
}
