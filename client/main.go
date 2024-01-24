package main

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/noartem/ecdh"

	"github.com/gorilla/websocket"
)

const (
	CONN_HOST = "localhost"
	CONN_PORT = "4040"
	CONN_TYPE = "tcp"
)

type Payload struct {
	Id      string `json:"id"`
	Message string `json:"message"`
}

type MsgSchema struct {
	Type int `json:"type"`
	Key []byte `json:"key"`
	Sender string `json:"sender"`
	Payload Payload `json:"payload"`
}

func padding(m []byte) []byte {
	l := len(m)
	return m[:(l - int(m[l-1]))]
}

// -------------------------------------------------------------------------------- //

func send_message(w *websocket.Conn, m MsgSchema) error {
	b, error := json.Marshal(m)
	if error != nil {
		log.Println("Marshalling error\n")
	}

	err := w.WriteMessage(websocket.TextMessage, b)
	if err != nil {
		log.Println("write:", err)
		return err
	}
	return nil
}

// -------------------------------------------------------------------------------- //

func encrypt(input_message string, peer string, aes_iv string) (string, error) {
	length := len(input_message)
	var plainTextBlock []byte

	if length%16 != 0 {
		extendBlock := 16 - (length % 16)
		plainTextBlock = make([]byte, length+extendBlock)
		copy(plainTextBlock[length:], bytes.Repeat([]byte{uint8(extendBlock)}, extendBlock))
	} else {
		plainTextBlock = make([]byte, length)
	}

	copy(plainTextBlock, input_message)
	enc_outgoing_msg := make([]byte, len(plainTextBlock))
	
	block, err := aes.NewCipher([]byte(peer))
	if err != nil {
		return "", err
	}
	mode := cipher.NewCBCEncrypter(block, []byte(aes_iv))
	mode.CryptBlocks(enc_outgoing_msg, plainTextBlock)
	return base64.StdEncoding.EncodeToString(enc_outgoing_msg), nil
}

// -------------------------------------------------------------------------------- //

func decrypt(key string, sender string, message string, aes_iv string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		fmt.Printf("Could not decrypt message for sender: %s", sender)
		return "", err
	}
	enc_incoming_msg := []byte(message)
	ctext, err := base64.StdEncoding.DecodeString(string(enc_incoming_msg))
	mode := cipher.NewCBCDecrypter(block, []byte(aes_iv))
	mode.CryptBlocks(ctext, ctext)
	return string(padding(ctext)), nil
}

func main() {
	u := url.URL{Scheme: "ws", Host: CONN_HOST+":"+CONN_PORT, Path: "/ws"}
	log.Printf("Connecting to %s\n", u.String())
     
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}

	close_program := make(chan bool)
	defer c.Close()

	done := make(chan struct{})

	var inc_msg MsgSchema
	var set_peer int
	var set_peer_id string
	var input_message string

	// Map of clieint_uuid: aes_cipher 
	available_clients := make(map[string]string)

	kv_pair, err := ecdh.GenerateKey()
	if err != nil {
		fmt.Println("Could not initialize Diffie-Helman keys")
		log.Fatal()
	}
	// Key pair for Diffie-Helman algorithm
	_, pub_key := kv_pair.Marshal() 

	// The AES initialization vector
	aes_iv := "16bytetIniVKey12"
	// Current client's uuid returned from the server
  my_uuid := ""

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			split_message := strings.Split(string(message), "\n")
			
			for _, splt_msg := range(split_message) {
				err = json.Unmarshal([]byte(splt_msg), &inc_msg);			
					
				if err != nil {	
					log.Fatalf("Error response from server")
					continue
				}
	
				if inc_msg.Type == 101 {
					if strings.HasPrefix(inc_msg.Payload.Message, "(me)") {
						my_uuid = inc_msg.Sender
					}

					m := MsgSchema{Type: 102, Key: []byte(pub_key), Sender: my_uuid, Payload: Payload{Id: "", Message: ""},}
					err = send_message(c, m)
					if err != nil {
						continue
					}
				} else if inc_msg.Type == 102 {
						incomig_pub_key, err := ecdh.UnmarshalPublic(string(inc_msg.Key))
						if err != nil {
							fmt.Printf("Could not read incoming public key: %s", inc_msg.Key)
							continue
						}
						shared_secret, err := kv_pair.GenerateSecret(incomig_pub_key)
						if err != nil {
							fmt.Printf("Could not read decode public key: %s", inc_msg.Key)
							continue
						}
						available_clients[inc_msg.Sender] = shared_secret.Secret
				} else {
					ctext, err := decrypt(available_clients[inc_msg.Sender], inc_msg.Sender, inc_msg.Payload.Message, aes_iv)
					if err != nil {
						continue
					}
					fmt.Printf("From: %s --------------> incoming message: %s\n",inc_msg.Sender, string(ctext))
				}
				fmt.Printf("\n")
			}
		}
	}()

	var peers_list []string
	var peers_ids_list []string
	var peers_index int
	in := bufio.NewReader(os.Stdin)

	go func() {
		for {
			if len(available_clients) > 0 {	
				fmt.Printf("Choose a peer to send message to or press -1 to refetch peers:\n")
				peers_list = nil
				peers_ids_list = nil
				peers_index = 0
				for k, v := range(available_clients) {
					peers_list = append(peers_list, v)
					peers_ids_list = append(peers_ids_list, k)
					fmt.Printf("%d: %s\n", peers_index, k)
					peers_index+=1
				}
				fmt.Scanln(&set_peer)
				if set_peer < 0 {continue}

				if my_uuid == "" {
					fmt.Println("Your id is not published. Refetching peers.")
					continue
				}

				if set_peer >= len(peers_list) {
					fmt.Printf("Could not sent client with index: %d\n", set_peer)
					continue
				}

				set_peer_id = peers_ids_list[set_peer]
				fmt.Print("Give message:")
				input_message, err = in.ReadString('\n')
				
				set_peer_id = strings.ToLower(set_peer_id)

				prepared_string, err := encrypt(input_message, peers_list[set_peer], aes_iv)
				if err != nil {
					fmt.Println("Could not encrypt message. Sending blank")
				}

				m := MsgSchema{Type: 100, Key: []byte(pub_key), Sender: my_uuid, Payload: Payload{Id: set_peer_id, Message: prepared_string},}
				err = send_message(c,m)
				if err != nil {
					fmt.Println("Error sending message. Please re-run the client")
					close_program <- true
				}

			}
		}
	}()

	<- close_program
	os.Exit(0)
}
