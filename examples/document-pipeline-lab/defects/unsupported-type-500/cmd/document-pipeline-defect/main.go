package main

import (
	"flag"
	"log"
	"net/http"

	unsupportedtype500 "ingen/examples/document-pipeline-lab/defects/unsupported-type-500"
	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func main() {
	address := flag.String("addr", ":8083", "HTTP listen address")
	flag.Parse()

	subject := documentpipeline.NewHandler(documentpipeline.NewStore())
	log.Printf("unsupported-type-500 defect subject listening on %s", *address)
	if err := http.ListenAndServe(*address, unsupportedtype500.NewHandler(subject)); err != nil {
		log.Fatal(err)
	}
}
