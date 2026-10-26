package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/audit"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(audit.New()); err != nil {
		log.Fatal(err)
	}
}
