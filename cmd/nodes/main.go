package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/nodes"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(nodes.New()); err != nil {
		log.Fatal(err)
	}
}
