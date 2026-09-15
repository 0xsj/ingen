package main

import (
	"flag"
	"log"
	"net/http"

	persistencewrongkey "ingen/examples/document-pipeline-lab/defects/persistence-wrong-key"
	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

func main() {
	address := flag.String("addr", ":8085", "HTTP listen address")
	flag.Parse()

	subject := documentpipeline.NewHandler(documentpipeline.NewStore())
	log.Printf("persistence-wrong-key defect subject listening on %s", *address)
	if err := http.ListenAndServe(*address, persistencewrongkey.NewHandler(subject)); err != nil {
		log.Fatal(err)
	}
}
