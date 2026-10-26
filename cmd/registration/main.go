package main

import (
	"log"

	"github.com/MoeClub-Open/PxBoard-Plugins/internal/registration"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func main() {
	if e := plugin.Serve(registration.New()); e != nil {
		log.Fatal(e)
	}
}
