package main

import (
	"github.com/MoeClub-Open/PxBoard-Plugins/internal/captcha"
	"github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
)

func main() {
	if err := plugin.Serve(captcha.New()); err != nil {
		log.Fatal(err)
	}
}
