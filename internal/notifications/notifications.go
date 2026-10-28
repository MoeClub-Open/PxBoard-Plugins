package notifications

import (
	"net/http"
	"sync"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type app struct {
	*c.App
	delivery  sync.Mutex
	lastSent  time.Time
	campaigns sync.Mutex
}

func New() http.Handler {
	a := &app{App: c.New()}
	a.Mux.HandleFunc("GET /campaigns", a.campaignList)
	a.Mux.HandleFunc("POST /campaigns", a.saveCampaign)
	a.Mux.HandleFunc("GET /campaigns/{id}", a.getCampaign)
	a.Mux.HandleFunc("PATCH /campaigns/{id}", a.saveCampaign)
	a.Mux.HandleFunc("POST /campaigns/{id}/{action}", a.campaignAction)
	a.Mux.HandleFunc("GET /mail", a.mailConfig)
	a.Mux.HandleFunc("PATCH /mail", a.mailConfig)
	a.Mux.HandleFunc("GET /templates", a.templates)
	a.Mux.HandleFunc("GET /templates/{id}", a.templateAction)
	a.Mux.HandleFunc("PATCH /templates/{id}", a.templateAction)
	a.Mux.HandleFunc("POST /templates/{id}/{action}", a.templateAction)
	a.Mux.HandleFunc("GET /notifications", a.notifications)
	a.Mux.HandleFunc("POST /notifications/{id}/retry", a.retryNotification)
	a.Mux.HandleFunc("GET /_capabilities", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r, "core") {
			return
		}
		rows, err := a.C.List("mail")
		if !c.Err(w, err) {
			p.JSON(w, map[string]bool{"auth.password-reset": len(rows) > 0 && p.Bool(rows[0], "enabled")})
		}
	})
	a.View("notifications", a.notificationView)
	a.Events(a.event)
	return a
}
