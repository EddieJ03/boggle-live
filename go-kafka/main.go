package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"sync"

	"github.com/segmentio/kafka-go"

	"math/rand"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rs/cors"
)

var debug bool

// change this endpoint depending on where server is (ex: localhost:9094)
var endpoint string = "37.117.12.142:9094"

func init() {
	// Initialize the debug flag from the command line arguments
	flag.BoolVar(&debug, "debug", false, "Enable debug mode")
	flag.Parse()
}

type WSClient struct {
	Conn           *websocket.Conn
	UniqueNumber   int
	MessageChan    chan map[string]interface{}
	WaitGroup      sync.WaitGroup
}

func topicExists(topic string) (bool, error) {
	// Connect to the Kafka broker
	conn, err := kafka.DialContext(context.Background(), "tcp", endpoint)
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

func (c * WSClient) newSpectator(ctx context.Context, topic string) {
	debugPrint(fmt.Sprintf("NEW spectator for topic %s \n", topic))

    r := kafka.NewReader(kafka.ReaderConfig{
        Brokers:   []string{endpoint},
        Topic:     topic,
        Partition: 0,
        MaxBytes:  10e6, 
    })

	c.WaitGroup.Add(1)
	
	defer debugPrint(fmt.Sprintf("FINISHED spectator for topic %s \n", topic))
	defer r.Close()
	defer c.WaitGroup.Done()

    for {
        select {
        case <-ctx.Done():
			debugPrint("EXITING saw context finished")
            return // Exit gracefully if context is canceled
        default:
            m, err := r.ReadMessage(ctx)
			
            if err != nil {
				if errors.Is(err, context.Canceled) {
					debugPrint("EXITING saw context CANCELLED ERROR")
					return
				}

				c.MessageChan <- map[string]interface{}{
					"type":    "error",
					"message": "Failed to read from game " + topic + "!",
				}

                return // Exit on error
            }

            c.MessageChan <- map[string]interface{}{
                "type":    "message",
                "message": string(m.Value),
				"topic": topic,
            }

			if strings.Contains(string(m.Value), "GAME OVER!") { // Exit on "GAME OVER!"
				return
			}
        }
    }
}


func (c *WSClient) HandleClient() {
    defer c.Conn.Close()
	debugPrint(fmt.Sprintf("%d connected\n", c.UniqueNumber))

	ctx, cancel := context.WithCancel(context.Background())

	c.Conn.SetCloseHandler(func(code int, text string) error {
		debugPrint(fmt.Sprintf("%d closed so disconnected\n", c.UniqueNumber))
		cancel()
		c.WaitGroup.Wait()
		close(c.MessageChan)
		return nil
	})

	// pipe all messages through a dedicated channel (best if message order is needed, but not necessary for my implementation)
	go func() {
		debugPrint("starting infinite read from message channel\n")
        for message := range c.MessageChan {
            if err := c.Conn.WriteJSON(message); err != nil {
                debugPrint(fmt.Sprintf("WebSocket write error: %v", err))
                cancel() // Stop all current Kafka readers on write failure since no point in reading

				// however we can keep looping in case write succeeds later
            }
        }
		debugPrint("finished infinite read from message channel\n")
    }()

	for {
		select {
		case <-ctx.Done():
			debugPrint(fmt.Sprintf("%d context canceled so disconnecting\n", c.UniqueNumber))
        	return
		default:
			var data map[string]interface{}

			err := c.Conn.ReadJSON(&data)
			if err != nil && websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				debugPrint(fmt.Sprintf("application or protocol error so disconnected %v\n", err))

				cancel()
				c.WaitGroup.Wait()
				close(c.MessageChan)
				
				return
			}

			msgType, ok := data["type"].(string)
			if !ok {
				debugPrint(fmt.Sprintf("%s is invalid type for message Type\n", msgType))
				continue
			}

			switch msgType {
			case "newSpectator":
				exists, err := topicExists(data["topic"].(string))

				if err != nil {
					c.Conn.WriteJSON(map[string]interface{}{
						"type":   "error",
						"message": "Failed to check if " + data["topic"].(string) + " exists. Kafka cluster likely down.",
					})
					continue
				}

				if exists {
					go c.newSpectator(ctx, data["topic"].(string))
				} else {
					c.Conn.WriteJSON(map[string]interface{}{
						"type":   "error",
						"message": "Game " + data["topic"].(string) + " does not exist!",
					})
				}
				
			default:
				continue
			}
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
	if debug {
		fmt.Println("Debug mode enabled")
	} else {
		fmt.Println("Debug mode not enabled")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleConnections)
	handler := cors.Default().Handler(mux)

	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}

	infoPrint("Server is running on port 8080!")

	server.ListenAndServe()
}

func debugPrint(message string) {
	if debug {
		fmt.Println("DEBUG:", message)
	}
}

func infoPrint(message string) {
	fmt.Println("INFO:", message) // Always prints
}
