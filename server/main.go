package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	fmt.Println("Go Websocket")
	flag.Parse()
	hub := NewHub()
	go hub.run()
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r)
	})
	log.Fatal(http.ListenAndServe(":4040", nil))
}
