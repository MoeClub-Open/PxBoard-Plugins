package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/commerce"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(commerce.New()); err != nil {
		log.Fatal(err)
	}
}
