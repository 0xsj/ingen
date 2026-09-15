package main

import (
	"flag"
	"log"
	"net/http"

	processstaysqueued "ingen/examples/document-pipeline-lab/defects/process-stays-queued"
	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func main() {
	address := flag.String("addr", ":8084", "HTTP listen address")
	flag.Parse()

	subject := documentpipeline.NewHandler(documentpipeline.NewStore())
	log.Printf("process-stays-queued defect subject listening on %s", *address)
	if err := http.ListenAndServe(*address, processstaysqueued.NewHandler(subject)); err != nil {
		log.Fatal(err)
	}
}
