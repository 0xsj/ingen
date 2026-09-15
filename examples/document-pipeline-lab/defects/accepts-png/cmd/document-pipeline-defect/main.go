package main

import (
	"flag"
	"log"
	"net/http"

	acceptspng "ingen/examples/document-pipeline-lab/defects/accepts-png"
	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func main() {
	address := flag.String("addr", ":8086", "HTTP listen address")
	flag.Parse()

	subject := documentpipeline.NewHandler(documentpipeline.NewStore())
	log.Printf("accepts-png defect subject listening on %s", *address)
	if err := http.ListenAndServe(*address, acceptspng.NewHandler(subject)); err != nil {
		log.Fatal(err)
	}
}
