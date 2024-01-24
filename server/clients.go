package main

import (
	"bytes"
	"log"
	"net/http"
	"time"

	"encoding/json"

	"github.com/gorilla/websocket"
	uuid "github.com/nu7hatch/gouuid"
)

const (
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Client struct {
	hub *Hub
	conn *websocket.Conn
	send chan []byte
	id *uuid.UUID
}

type Payload struct {
	Id      string `json:"id"`
	Message string `json:"message"`
}

type OutgoingMessage struct {
	Type int  `json:"type"`
	Key []byte `json:"key"`
	Sender string `json:"sender"`
	Payload Payload `json:"payload"`
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message, newline, space, -1))
		c.hub.broadcast <- message
	}
}

func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for {
		select {
		case message := <-c.send:
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		}
	}
}

func FormatInitMessage(msg_type int,  sender string, msg string, receiverId string) []byte {
	outgoing_message := &OutgoingMessage{
		Type: msg_type,
		Key: []byte(""),
		Sender: sender,
		Payload: Payload{
			Id: receiverId,
			Message: msg,
		},
	}

	byte_outgoing_message, err := json.Marshal(outgoing_message)
	if err != nil {
		log.Print("Error encoding outgoing message")
		return []byte("")
	}
	return byte_outgoing_message
}

func BroadcastAvailableConnections(current_id string, hub *Hub, ids []*uuid.UUID) {
	log.Println(ids)
	for _, id := range ids {
		if (current_id == id.String()) && (current_id != "") {
			hub.broadcast <- FormatInitMessage(101, current_id, "(me) " + current_id, id.String())
		} else {
			hub.broadcast <- FormatInitMessage(101, current_id, current_id, id.String())
		}
	}
}

func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	u, err := uuid.NewV4()
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256), id: u}
	client.hub.register <- client

	time.Sleep(1 * time.Second) 
	log.Print("Broadcasting new id to peers")
	BroadcastAvailableConnections(u.String(), hub, hub.clientsIds)
	go client.writePump()
	go client.readPump()
}
