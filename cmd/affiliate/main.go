package main

import (
	"log"

	"github.com/MoeClub-Open/PxBoard-Plugins/internal/affiliate"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func main() {
	if e := p.Serve(affiliate.New()); e != nil {
		log.Fatal(e)
	}
}
