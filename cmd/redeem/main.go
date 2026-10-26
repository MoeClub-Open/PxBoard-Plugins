package main

import (
	"log"

	"github.com/MoeClub-Open/PxBoard-Plugins/internal/redeem"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func main() {
	if err := plugin.Serve(redeem.New()); err != nil {
		log.Fatal(err)
	}
}
