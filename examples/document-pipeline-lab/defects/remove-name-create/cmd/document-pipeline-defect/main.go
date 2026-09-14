package main

import (
	"flag"
	"log"
	"net/http"

	removenamecreate "ingen/examples/document-pipeline-lab/defects/remove-name-create"
	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func main() {
	address := flag.String("addr", ":8082", "HTTP listen address")
	flag.Parse()

	subject := documentpipeline.NewHandler(documentpipeline.NewStore())
	log.Printf("remove-name-create defect subject listening on %s", *address)
	if err := http.ListenAndServe(*address, removenamecreate.NewHandler(subject)); err != nil {
		log.Fatal(err)
	}
}
