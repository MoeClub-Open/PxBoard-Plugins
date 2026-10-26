package main

import (
	"log"

	"github.com/MoeClub-Open/PxBoard-Plugins/internal/oauth"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func main() {
	if err := plugin.Serve(oauth.New()); err != nil {
		log.Fatal(err)
	}
}
