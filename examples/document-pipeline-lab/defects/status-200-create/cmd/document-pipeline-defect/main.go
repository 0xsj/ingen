package main

import (
	"flag"
	"log"
	"net/http"

	status200create "ingen/examples/document-pipeline-lab/defects/status-200-create"
	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func main() {
	address := flag.String("addr", ":8081", "HTTP listen address")
	flag.Parse()

	subject := documentpipeline.NewHandler(documentpipeline.NewStore())
	log.Printf("status-200-create defect subject listening on %s", *address)
	if err := http.ListenAndServe(*address, status200create.NewHandler(subject)); err != nil {
		log.Fatal(err)
	}
}
