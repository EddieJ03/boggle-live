package main

import (
	"fmt"
	"unicode/utf8"

	"boggle-backend/boards"
	"boggle-backend/trie"

	"github.com/gorilla/websocket"
)

type WSClient struct {
	Conn           *websocket.Conn
	RoomName       string
	UniqueNumber   int
	Number         int
}

func (c *WSClient) HandleClient() {
	defer c.Conn.Close()
	fmt.Printf("%d connected\n", c.UniqueNumber)

	c.Conn.SetCloseHandler(func(code int, text string) error {
		c.handleDisconnect()
		fmt.Printf("%d closed so disconnected\n", c.UniqueNumber)
		return nil
	})

	for {
		var data map[string]interface{}

		err := c.Conn.ReadJSON(&data)
		if err != nil {
			fmt.Println(err)

			// disconnect both when error
			c.handleDisconnect()

			fmt.Printf("%d error so disconnected\n", c.UniqueNumber)

			break
		}

		msgType, ok := data["type"].(string)
		if !ok {
			// fmt.Printf("%s is invalid type for message Type\n", msgType)
			continue
		}

		// fmt.Println("messageType: " + msgType)

		switch msgType {
		case "newGame":
			c.newGame(false)
		case "joinGame":
			c.joinGame(data["roomName"].(string))
		case "submitWord":
			swm := SubmitWordMessage{
				Type: data["type"].(string),
				Word: data["word"].(string),
				Score: data["score"].(float64),
			}

			c.submitWord(swm)
		case "randomGame":
			c.randomGame()
		default:
			continue
		}
	}
}


func (c *WSClient) newGame(random bool) {
	var commonTrie = trie.NewTrie()

	for item := range boards.Common {
		commonTrie.Add(item)
	}

	roomName := makeID(15)

	c.RoomName = roomName
	c.Number = 1

	if(!random) {
		c.Conn.WriteJSON(map[string]string{
			"type":     "gameCode",
			"roomName": roomName,
		})
	} else {
		c.Conn.WriteJSON(map[string]string{
			"type":     "randomWaiting",
			"roomName": roomName,
		})
	}

	initGame(roomName, commonTrie, random)

	clientRoomsLock.RLock()
	defer clientRoomsLock.RUnlock()

	room, exists := clientRooms[roomName]
	if !exists {
		return
	}

	room.Player1WS = c

	c.Conn.WriteJSON(map[string]interface{}{
		"type":   "init",
		"number": 1,
	})

	fmt.Printf("%d is player %d in room %s\n", c.UniqueNumber, c.Number, c.RoomName)
}


func (c *WSClient) joinGame(roomName string) {
	clientRoomsLock.RLock()
	defer clientRoomsLock.RUnlock()

	room, exists := clientRooms[roomName]
	if !exists {
		c.Conn.WriteJSON(map[string]string{
			"type": "unknownGame",
		})
		return
	}

	// one new player at a time should be here
	room.RoomLock.Lock()

	numClients := numberOfClients(room)

	if numClients == 0 || numClients == -1 {
		room.RoomLock.Unlock()
		// fmt.Println("Room " + roomName + " has 0 players??!")
		c.Conn.WriteJSON(map[string]string{
			"type": "unknownGame",
		})
		return
	} else if numClients > 1 {
		room.RoomLock.Unlock()
		// fmt.Println("Room " + roomName + " has too many players??!")
		c.Conn.WriteJSON(map[string]string{
			"type": "tooManyPlayers",
		})
		return
	}

	c.Number = 2
	c.RoomName = roomName
	room.Player2WS = c

	c.Conn.WriteJSON(map[string]interface{}{
		"type":   "init",
		"number": 2,
	})

	fmt.Printf("%d is player %d in room %s\n", c.UniqueNumber, c.Number, c.RoomName)
	
	room.RoomLock.Unlock()

	startGame(room)
}

func (c *WSClient) submitWord(data SubmitWordMessage) {
	clientRoomsLock.RLock()

	room, exists := clientRooms[c.RoomName]
	if !exists {
		return
	}

	clientRoomsLock.RUnlock()

    if c.Number == 1 {
        // fmt.Println("Switching to player 2")

		if utf8.RuneCountInString(data.Word) == 0 {
			room.Player1MissedTurns += 1
		} else {
			room.Player1MissedTurns = 0
		}

        room.Player1 = data.Score

		if room.Player1MissedTurns == 3 || room.Player1 + room.Player2 == float64(room.TotalScore) {
			broadcastEndGame(room, room.Player1, room.Player2)
		}

        broadcastSwitch(c.RoomName, 1, 2, data.Word)
    } else {
        // fmt.Println("Switching to player 1")

		if utf8.RuneCountInString(data.Word) == 0 {
			room.Player2MissedTurns += 1
		} else {
			room.Player2MissedTurns = 0
		}

        room.Player2 = data.Score

		if room.Player2MissedTurns == 3 || room.Player1 + room.Player2 == float64(room.TotalScore) {
			broadcastEndGame(room, room.Player1, room.Player2)
		}

        broadcastSwitch(c.RoomName, 2, 1, data.Word)
    }
}

func (c *WSClient) handleDisconnect() {
	if c.RoomName == "" {
		fmt.Printf("%d could not find room %s to delete after disconnect!\n", c.Number, c.RoomName)
		return
	}

	broadcastDisconnect(c.RoomName)
	
	clientRoomsLock.Lock()	

	delete(clientRooms, c.RoomName)

	clientRoomsLock.Unlock()

	randomRoomsLock.Lock()

	// also remove from randomRooms (if exists)
	randomRooms = removeRoom(randomRooms, findRoomIndex(randomRooms, c.RoomName))

	randomRoomsLock.Unlock()

	fmt.Printf("%d found room %s to delete after disconnect!\n", c.Number, c.RoomName)
}

func (c * WSClient) randomGame() {
	// hold lock to randomRooms
	randomRoomsLock.Lock()

	if len(randomRooms) == 0 {
		c.newGame(true)
		randomRoomsLock.Unlock()
	} else {
		// pull from list and start random game!
		var randomRoom *Room = nil

		randomRooms, randomRoom = popFirstRoom(randomRooms)

		randomRoomsLock.Unlock()

		c.joinGame(randomRoom.RoomName)
	}
}