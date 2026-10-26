package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptions"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(subscriptions.New()); err != nil {
		log.Fatal(err)
	}
}
