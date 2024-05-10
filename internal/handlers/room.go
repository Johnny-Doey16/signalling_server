package handlers

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/steve-mir/diivix_signalling_server/pkg/chat"
	w "github.com/steve-mir/diivix_signalling_server/pkg/webrtc"

	"crypto/sha256"

	"github.com/jaevor/go-nanoid"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"

	// guuid "github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

func RoomCreate(c *fiber.Ctx) error {
	// return c.Redirect(fmt.Sprintf("/room/%s", guuid.New().String()))
	uuid, err := generateRoomId() //guuid.New().String()
	if uuid == "" {
		c.Status(400)
		fmt.Println("Error creating uid", err)
		return nil
	}
	// uuid := c.Query("room_id")

	roomName := c.Query("name")
	if roomName == "" {
		c.Status(400)
		return nil
	}
	uid, suid, rm := createRoom(roomName, uuid)

	c.JSON(fiber.Map{
		"roomId": uid,
		"suuid":  suid,
		"room":   rm,
	})
	// Add roomId to room map
	return nil
}

func RoomCreateWithUid(c *fiber.Ctx) error {
	// uuid := c.Params("uuid")
	// if uuid == "" {
	// 	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no uid found"})
	// }
	uuid := c.Query("uuid")
	if uuid == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no uid found"})
	}

	roomName := c.Query("name")
	if roomName == "" {
		c.Status(400)
		return nil
	}
	uid, suid, rm := createRoom(roomName, uuid)
	log.Println("ID", uid, "room", rm)

	c.JSON(fiber.Map{
		"roomId": uid,
		"suuid":  suid,
		"room":   rm,
	})
	// Add roomId to room map
	return nil
}

func Room(c *fiber.Ctx) error {
	uuid := c.Params("uuid")
	if uuid == "" {
		c.Status(400)
		return nil
	}

	ws := "ws"
	if os.Getenv("ENVIRONMENT") == "PRODUCTION" {
		ws = "wss"
	}

	_, suuid, _, err := getRoom(uuid) //createOrGetRoom(uuid)

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Room not found"})
	}
	return c.JSON(fiber.Map{
		"RoomWebsocketAddr":   fmt.Sprintf("%s://%s/room/%s/websocket", ws, c.Hostname(), uuid),
		"RoomLink":            fmt.Sprintf("%s://%s/room/%s", c.Protocol(), c.Hostname(), uuid),
		"ChatWebsocketAddr":   fmt.Sprintf("%s://%s/room/%s/chat/websocket", ws, c.Hostname(), uuid),
		"ViewerWebsocketAddr": fmt.Sprintf("%s://%s/room/%s/viewer/websocket", ws, c.Hostname(), uuid),
		"StreamLink":          fmt.Sprintf("%s://%s/stream/%s", c.Protocol(), c.Hostname(), suuid),
		"Type":                "room",
		"RoomName":            w.Rooms[uuid].Name,
		"RoomDetails":         w.Rooms[uuid],
	})

}

func RoomWebsocket(c *websocket.Conn) {
	uuid := c.Params("uuid")
	if uuid == "" {
		return
	}

	_, _, room, err := getRoom(uuid) //createOrGetRoom(uuid)
	if err != nil {
		fmt.Println("Room doesn't exist", err)
		return
	}

	// room.User = &w.User{
	// 	StreamID: "someStreamId",
	// 	Email:    "user@example.com",
	// 	Avatar:   "https://example.com/avatar.jpg",
	// 	Username: "JohnDoe",
	// }

	// w.RoomConn(c, room.Peers, room.User[0])
	w.RoomConn(c, room)
}

func RoomViewerWebsocket(c *websocket.Conn) {
	uuid := c.Params("uuid")
	if uuid == "" {
		return
	}

	w.RoomsLock.Lock()
	if peer, ok := w.Rooms[uuid]; ok {
		w.RoomsLock.Unlock()
		roomViewerConn(c, peer.Peers)
		return
	}
	w.RoomsLock.Unlock()
}

func roomViewerConn(c *websocket.Conn, p *w.Peers) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer c.Close()

	for {
		select {
		case <-ticker.C:
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write([]byte(fmt.Sprintf("%d", len(p.Connections))))
		}
	}
}

type websocketMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

func generateRoomId() (string, error) {
	// Generate a 11-character string.
	decenaryID, err := nanoid.CustomASCII("abcdefghijklmnopqrstuvwxyz", 11)
	if err != nil {
		return "", err
	}

	id := decenaryID()
	// Insert hyphens to match the pattern "abcs-def-hij".
	formattedID := fmt.Sprintf("%s-%s-%s", id[0:4], id[4:7], id[7:11])

	fmt.Printf("Formatted ID: %s\n", formattedID)
	return formattedID, nil
}

// createRoom creates a new room or returns an existing one based on the given UUID.
func createRoom(name, uuid string) (string, string, *w.Room) {
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	h := sha256.New()
	h.Write([]byte(uuid))
	suuid := fmt.Sprintf("%x", h.Sum(nil))

	if room := w.Rooms[uuid]; room != nil {
		if _, ok := w.Streams[suuid]; !ok {
			w.Streams[suuid] = room
		}
		return uuid, suuid, room
	}

	return createNewRoom(name, uuid, suuid)
}

// getRoom retrieves an existing room based on the given UUID.
// If the room doesn't exist, it returns an error.
func getRoom(uuid string) (string, string, *w.Room, error) {
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	h := sha256.New()
	h.Write([]byte(uuid))
	suuid := fmt.Sprintf("%x", h.Sum(nil))

	if room := w.Rooms[uuid]; room != nil {
		if _, ok := w.Streams[suuid]; !ok {
			w.Streams[suuid] = room
		}
		return uuid, suuid, room, nil
	}

	return "", "", nil, errors.New("Room not found")
}

// createNewRoom is a helper function to create a new room.
func createNewRoom(name, uuid, suuid string) (string, string, *w.Room) {
	hub := chat.NewHub()
	p := &w.Peers{}
	p.TrackLocals = make(map[string]*webrtc.TrackLocalStaticRTP)
	room := &w.Room{
		Name:  name,
		Peers: p,
		Hub:   hub,
	}

	w.Rooms[uuid] = room
	w.Streams[suuid] = room

	go hub.Run()
	return uuid, suuid, room
}

func GetUserFromRoom(c *fiber.Ctx) error {
	roomId := c.Query("roomId")
	streamId := c.Query("streamId")

	if roomId == "" || streamId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "roomId and streamId are required",
		})
	}

	user, err := getUserDetailsFromRoom(roomId, streamId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(user)
}

func getUserDetailsFromRoom(roomId, streamId string) (*w.User, error) {
	w.RoomsLock.RLock()
	defer w.RoomsLock.RUnlock()

	room, found := w.Rooms[roomId]
	if !found {
		return nil, fmt.Errorf("room with ID %s not found", roomId)
	}

	user, found := room.User[streamId]
	if !found {
		return nil, fmt.Errorf("user with stream ID %s not found in room %s", streamId, roomId)
	}

	return user, nil
}

func GetUserFromRoom2(c *fiber.Ctx) error {
	roomId := c.Query("roomId")

	if roomId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "roomId is required",
		})
	}

	users, err := getUserMapFromRoom2(roomId)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(users)
}

func getUserMapFromRoom2(roomId string) (map[string]*w.User, error) {
	w.RoomsLock.RLock()
	defer w.RoomsLock.RUnlock()

	room, found := w.Rooms[roomId]
	if !found {
		return nil, fmt.Errorf("room with ID %s not found", roomId)
	}

	return room.User, nil
}

func createOrGetRoom(uuid string) (string, string, *w.Room) {
	w.RoomsLock.Lock()
	defer w.RoomsLock.Unlock()

	h := sha256.New()
	h.Write([]byte(uuid))
	suuid := fmt.Sprintf("%x", h.Sum(nil))

	if room := w.Rooms[uuid]; room != nil {
		if _, ok := w.Streams[suuid]; !ok {
			w.Streams[suuid] = room
		}
		return uuid, suuid, room
	}

	hub := chat.NewHub()
	p := &w.Peers{}
	p.TrackLocals = make(map[string]*webrtc.TrackLocalStaticRTP)
	room := &w.Room{
		Peers: p,
		Hub:   hub,
	}

	w.Rooms[uuid] = room
	w.Streams[suuid] = room

	go hub.Run()
	return uuid, suuid, room
}

/*
func generateRoomId() string {
	// u := guuid.New()
	// hasher := sha1.New()
	// hasher.Write(u[:])
	// sha := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

	// // Truncate to the desired length
	// shortID := sha[:12] // Adjust length as needed, keeping in mind the collision risk

	// fmt.Println("Original UUID:", u)
	// fmt.Println("Short ID:", shortID)
	// return shortID
	id, err := gonanoid.New()
	if err != nil {
		// handle error
		fmt.Println(err)
		return ""
	}
	fmt.Println("NanoID:", id)
	return id
}

func generateRoomId3() string {
	id := shortuuid.New() // or shortuuid.NewWithoutDashes()
	fmt.Println("Short UUID:", id)
	return id
}
func generateRoomId2Old() string {
	// The canonic NanoID is nanoid.Standard(21).
	// canonicID, err := nanoid.Standard(21)
	// if err != nil {
	// 	panic(err)
	// }

	// id1 := canonicID()
	// fmt.Printf("ID 1: %s", id1) // eLySUP3NTA48paA9mLK3V

	// Makes sense to use CustomASCII since 0-9 is ASCII.
	decenaryID, err := nanoid.CustomASCII("abcdefghijklmnopqrstuvwxyz", 10)
	if err != nil {
		panic(err)
	}

	id2 := decenaryID()
	fmt.Printf("ID 2: %s", id2)
	return id2
}
*/
