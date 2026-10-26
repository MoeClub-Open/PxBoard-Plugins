package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/migration"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(migration.New("v2board")); err != nil {
		log.Fatal(err)
	}
}
