package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/segmentio/kafka-go"

	"math/rand"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rs/cors"
)

type WSClient struct {
	Conn           *websocket.Conn
	UniqueNumber   int
}

func (c * WSClient) newSpectator(topic string) {
    r := kafka.NewReader(kafka.ReaderConfig{
        Brokers:   []string{"localhost:9094"},
        Topic:     topic,
        Partition: 0,
        MaxBytes:  10e6, // 10MB
    })

    for {
        m, err := r.ReadMessage(context.Background())

        c.Conn.WriteJSON(map[string]interface{}{
            "type":    "message",
            "message": string(m.Value),
        })

        if err != nil || strings.Contains(string(m.Value), "GAME OVER!") {
            break
        }
    }

    if err := r.Close(); err != nil {
        log.Fatal("failed to close reader:", err)
    }
}
func (c *WSClient) HandleClient() {
    defer c.Conn.Close()
	fmt.Printf("%d connected\n", c.UniqueNumber)

	c.Conn.SetCloseHandler(func(code int, text string) error {
		fmt.Printf("%d closed so disconnected\n", c.UniqueNumber)
		return nil
	})

	for {
		var data map[string]interface{}

		err := c.Conn.ReadJSON(&data)
		if err != nil {
			fmt.Println(err)

			fmt.Printf("%d error so disconnected\n", c.UniqueNumber)

			break
		}

		msgType, ok := data["type"].(string)
		if !ok {
			fmt.Printf("%s is invalid type for message Type\n", msgType)
			continue
		}

		fmt.Println("messageType: " + msgType)

		switch msgType {
		case "newSpectator":
			c.newSpectator(data["topic"].(string))
		default:
			continue
		}
	}
}

const NUM = 4

var upgrader = websocket.Upgrader{
	ReadBufferSize: 1024,
	WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        return true // Allow all origins
    },
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		fmt.Println(err)
		return
	}

	wsClient := &WSClient{
		Conn: conn, 
		UniqueNumber: rand.Int(), 
	}

	wsClient.HandleClient()
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleConnections)
	handler := cors.Default().Handler(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	fmt.Println("Server is running on port 8080!")

	server.ListenAndServe()
}