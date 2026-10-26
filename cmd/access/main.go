package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/access"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(access.New()); err != nil {
		log.Fatal(err)
	}
}
