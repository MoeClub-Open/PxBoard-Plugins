package main

import (
	"log"

	"github.com/MoeClub-Open/PxBoard-Plugins/internal/admob"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func main() {
	if err := plugin.Serve(admob.New()); err != nil {
		log.Fatal(err)
	}
}
