package server

import (
	"flag"
	"os"
	"time"

	// "github.com/steve-mir/diivix_signalling_server/cache"
	"github.com/steve-mir/diivix_signalling_server/internal/handlers"
	w "github.com/steve-mir/diivix_signalling_server/pkg/webrtc"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"

	// "github.com/gofiber/template/html"
	"github.com/gofiber/template/html/v2"
	"github.com/gofiber/websocket/v2"
)

var (
	addr = flag.String("addr", ":"+os.Getenv("PORT"), "")
	cert = flag.String("cert", "", "")
	key  = flag.String("key", "", "")
)

func Run() error {
	flag.Parse()

	if *addr == ":" {
		*addr = ":8080" //"192.168.0.182:8080" //
	}

	engine := html.New("./views", ".html")
	app := fiber.New(fiber.Config{Views: engine})
	app.Use(logger.New())
	// app.Use(cors.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
	}))

	// cache.Init() // initialize redis db

	// Room video chat
	app.Get("/", handlers.Welcome)
	app.Get("/room/create", handlers.RoomCreate)                // ! create a new room
	app.Get("/room/create_with_id", handlers.RoomCreateWithUid) // ! create a new room with a uid
	app.Get("/room/:uuid", handlers.Room)                       // ! Get a room details by id/link(uuid)
	app.Get("/room/:uuid/websocket", websocket.New(handlers.RoomWebsocket, websocket.Config{
		HandshakeTimeout: 10 * time.Second,
	})) // ! Join a room by id/link(uuid).
	app.Get("/room/user/details", handlers.GetUserFromRoom)   // ! Get user details in the room
	app.Get("/room/user/details2", handlers.GetUserFromRoom2) // ! Get user details in the room

	// room text chat
	app.Get("/room/:uuid/chat", handlers.RoomChat)
	app.Get("/room/:uuid/chat/websocket", websocket.New(handlers.RoomChatWebsocket))
	app.Get("/room/:uuid/viewer/websocket", websocket.New(handlers.RoomViewerWebsocket))
	app.Get("/stream/:suuid", handlers.Stream)
	app.Get("/stream/:suuid/websocket", websocket.New(handlers.StreamWebsocket, websocket.Config{
		HandshakeTimeout: 10 * time.Second,
	}))
	app.Get("/stream/:suuid/chat/websocket", websocket.New(handlers.StreamChatWebsocket))
	app.Get("/stream/:suuid/viewer/websocket", websocket.New(handlers.StreamViewerWebsocket))
	app.Static("/", "./assets")

	w.Rooms = make(map[string]*w.Room)
	w.Streams = make(map[string]*w.Room)
	go dispatchKeyFrames()
	if *cert != "" {
		return app.ListenTLS(*addr, *cert, *key)
	}
	return app.Listen(*addr)
}

func dispatchKeyFrames() {
	for range time.NewTicker(time.Second * 3).C {
		for _, room := range w.Rooms {
			room.Peers.DispatchKeyFrame()
		}
	}
}
