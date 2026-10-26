package main

import (
	"log"

	"github.com/MoeClub-Open/PxBoard-Plugins/internal/branding"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func main() {
	if err := plugin.Serve(branding.New()); err != nil {
		log.Fatal(err)
	}
}
