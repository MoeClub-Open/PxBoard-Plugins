package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/notifications"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(notifications.New()); err != nil {
		log.Fatal(err)
	}
}
