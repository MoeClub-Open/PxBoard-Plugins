package registration

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("POST /register", a.register)
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings", a.settings)
	a.Mux.HandleFunc("GET /internal/policy", a.policy)
	a.Mux.HandleFunc("GET /internal/registrations", a.registrations)
	a.Mux.HandleFunc("GET /_capabilities", a.capabilities)
	a.View("settings", a.view)
	a.Events(func(event p.Event) error {
		if event.Type != "system.tick" || event.Source != "core" {
			return fmt.Errorf("注册事件来源无效")
		}
		return a.RetryRegistrations("registrations")
	})
	return a
}

func (a *app) register(w http.ResponseWriter, r *http.Request) {
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	if !p.Bool(config, "enabled") {
		p.Fail(w, 403, "本站暂未开放注册")
		return
	}
	var b struct{ Email, Password, Name, Referral string }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	row := p.Record{"id": p.ID(), "registeredAt": c.Now(), "registrationPending": true}
	if referral := strings.TrimSpace(b.Referral); referral != "" {
		row["referral"] = referral
	}
	userID, e := a.C.CreateUserWithRecord(b.Email, b.Name, b.Password, "registrations", row)
	if c.Err(w, e) {
		return
	}
	row["userId"] = userID
	if e = a.PublishRegistration("registrations", row); e != nil {
		log.Printf("registration event pending for %s: %v", userID, e)
	}
	p.JSON(w, p.Record{"message": "账户已创建，请登录", "id": userID})
}

func (a *app) registrations(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "affiliate") {
		return
	}
	rows, e := a.C.List("registrations")
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, row := range rows {
		item := p.Record{"userId": row["userId"], "registeredAt": row["registeredAt"]}
		if referral := p.Text(row, "referral"); referral != "" {
			item["referral"] = referral
		}
		out = append(out, item)
	}
	p.JSON(w, out)
}
