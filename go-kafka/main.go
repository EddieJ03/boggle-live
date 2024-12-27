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
	MessageChan    chan map[string]interface{}
	CancelCtx 	   context.CancelFunc 
}

func (c * WSClient) newSpectator(ctx context.Context, topic string) {
    r := kafka.NewReader(kafka.ReaderConfig{
        Brokers:   []string{"localhost:9094"},
        Topic:     topic,
        Partition: 0,
        MaxBytes:  10e6, 
    })
	defer r.Close()

    for {
        select {
        case <-ctx.Done():
            return // Exit gracefully if context is canceled
        default:
            m, err := r.ReadMessage(ctx)

            c.MessageChan <- map[string]interface{}{
                "type":    "message",
                "message": string(m.Value),
				"topic": topic,
            }
			
            if err != nil || strings.Contains(string(m.Value), "GAME OVER!") {
                return // Exit on error or "GAME OVER!"
            }
        }
    }
}

func topicExists(topic string) (bool, error) {
	// Connect to the Kafka broker
	conn, err := kafka.DialContext(context.Background(), "tcp", "localhost:9094")
	if err != nil {
		return false, fmt.Errorf("failed to connect to Kafka broker: %w", err)
	}
	defer conn.Close()

	// Fetch metadata
	partitions, err := conn.ReadPartitions()
	if err != nil {
		return false, fmt.Errorf("failed to read partitions: %w", err)
	}

	// Check if the topic exists in the metadata
	for _, p := range partitions {
		if p.Topic == topic {
			return true, nil
		}
	}
	return false, nil
}

func (c *WSClient) HandleClient() {
    defer c.Conn.Close()
	fmt.Printf("%d connected\n", c.UniqueNumber)

	c.Conn.SetCloseHandler(func(code int, text string) error {
		fmt.Printf("%d closed so disconnected\n", c.UniqueNumber)
		close(c.MessageChan)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
    c.CancelCtx = cancel

	// pipe all messages through a dedicated channel (best if message order is needed, but not necessary for my implementation)
	go func() {
        for message := range c.MessageChan {
            if err := c.Conn.WriteJSON(message); err != nil {
                log.Println("WebSocket write error:", err)
                cancel() // Stop all Kafka readers on write failure since no point in reading
                break
            }
        }
    }()

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


		fmt.Println("topic: " + data["topic"].(string))
		fmt.Println("messageType: " + msgType)

		switch msgType {
		case "newSpectator":
			exists, err := topicExists(data["topic"].(string))

			if err != nil {
				fmt.Printf("Error checking topic existence: %v", err)
				continue
			}

			if exists {
				go c.newSpectator(ctx, data["topic"].(string))
			} else {
				c.Conn.WriteJSON(map[string]interface{}{
					"type":   "nonexistent",
					"topic":   data["topic"].(string),
				})
			}
			
		default:
			continue
		}
	}
}

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
		MessageChan: make(chan map[string]interface{}),
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