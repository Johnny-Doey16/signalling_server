package main

import (
	"log"

	"github.com/steve-mir/diivix_signalling_server/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		log.Fatalln(err.Error())
	}
}
