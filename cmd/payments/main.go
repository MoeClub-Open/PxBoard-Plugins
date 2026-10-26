package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/payments"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(payments.New()); err != nil {
		log.Fatal(err)
	}
}
