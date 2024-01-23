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

func PKCS5UnPadding(src []byte) []byte {
	length := len(src)
	unpadding := int(src[length-1])

	return src[:(length - unpadding)]
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
	available_clients := make(map[string]string)

	kv_pair, err := ecdh.GenerateKey()
	if err != nil {
		fmt.Println("Could not initialize dh keys")
		log.Fatal()
	}
	_, pub_key := kv_pair.Marshal() 


	aes_iv := "my16digitIvKey12"

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
					return
				}
	
				if inc_msg.Type == 101 {
					if strings.HasPrefix(inc_msg.Payload.Message, "(me)") {
						my_uuid = inc_msg.Sender
					}

					m := MsgSchema{Type: 102, Key: []byte(pub_key), Sender: my_uuid, Payload: Payload{Id: "", Message: ""},}
					b, error := json.Marshal(m)
					if error != nil {
						log.Println("Marshalling error\n")
					}
	
					err = c.WriteMessage(websocket.TextMessage, b)
					if err != nil {
						log.Println("write:", err)
						return
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
					block, err := aes.NewCipher([]byte(available_clients[inc_msg.Sender]))
					if err != nil {
						fmt.Printf("Could not decrypt message for sender: %s", inc_msg.Sender)
						continue
					}
					enc_incoming_msg := []byte(inc_msg.Payload.Message)
					ctext, err := base64.StdEncoding.DecodeString(string(enc_incoming_msg))
					if err != nil {
						continue
					}
					mode := cipher.NewCBCDecrypter(block, []byte(aes_iv))
					mode.CryptBlocks(ctext, ctext)
					ctext = PKCS5UnPadding(ctext)
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
				input_message = strings.ToLower(input_message)

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
				
				block, err := aes.NewCipher([]byte(peers_list[set_peer]))
				mode := cipher.NewCBCEncrypter(block, []byte(aes_iv))
				mode.CryptBlocks(enc_outgoing_msg, plainTextBlock)
				prepared_string := base64.StdEncoding.EncodeToString(enc_outgoing_msg)

				m := MsgSchema{Type: 100, Key: []byte(pub_key), Sender: my_uuid, Payload: Payload{Id: set_peer_id, Message: prepared_string},}
				b, error := json.Marshal(m)
				if error != nil {
					log.Println("Marshalling error\n")
				}

				err = c.WriteMessage(websocket.TextMessage, b)
				if err != nil {
					log.Println("write:", err)
					return
				}

				if strings.HasPrefix(string(b), "bye") {
				   fmt.Println("Good bye!")
				   close_program <- true
				}
			}
		}
	}()

	<- close_program
	os.Exit(0)
}
