package main

import (
	"encoding/json"
	"log"

	uuid "github.com/nu7hatch/gouuid"
)

type Hub struct {
	clients map[*Client]bool
	clientsIds []*uuid.UUID
	broadcast chan []byte
	register chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.clientsIds = append(h.clientsIds, client.id)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				if send_to_client(client, message) {
					select {
					case client.send <- message:
						var p OutgoingMessage
						err := json.Unmarshal(message, &p)
						if err != nil {
							log.Printf("Could not unmarshal message")
						}else {
							log.Printf("Peer: %s -> %s . Message: %s", p.Sender,  p.Payload.Id, p.Payload.Message)
						}
					default:
						close(client.send)
						delete(h.clients, client)
					}
				}
			}
		}
	}
}

func send_to_client(client *Client, message []byte) bool {
	var p OutgoingMessage
	err := json.Unmarshal(message, &p)
	if err != nil || p.Payload.Id == "" || p.Type == 102 {
		return true
	}
	return client.id.String() == p.Payload.Id
}
