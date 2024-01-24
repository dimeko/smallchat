package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

var port = ":4040"

func main() {
	fmt.Printf("Starting server on port: %s\n", port)
	flag.Parse()
	hub := NewHub()
	go hub.run()
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	})
	log.Fatal(http.ListenAndServe(port, nil))
}
