package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/sharing"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(sharing.New()); err != nil {
		log.Fatal(err)
	}
}
