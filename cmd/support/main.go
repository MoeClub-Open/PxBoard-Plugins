package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/support"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(support.New()); err != nil {
		log.Fatal(err)
	}
}
