package main

import (
	"flag"
	"log"
	"net/http"

	omitacceptedevent "ingen/examples/document-pipeline-lab/defects/omit-accepted-event"
	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func main() {
	address := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	subject := documentpipeline.NewHandler(documentpipeline.NewStore())
	log.Printf("omit-accepted-event defect subject listening on %s", *address)
	if err := http.ListenAndServe(*address, omitacceptedevent.NewHandler(subject)); err != nil {
		log.Fatal(err)
	}
}
