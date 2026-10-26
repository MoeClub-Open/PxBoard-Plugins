package oauth

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("GET /providers", a.providers)
	a.Mux.HandleFunc("GET /settings/providers", a.providerSettings)
	a.Mux.HandleFunc("POST /settings/providers", a.providerSettings)
	a.Mux.HandleFunc("PATCH /settings/providers/{id}", a.providerSettings)
	a.Mux.HandleFunc("DELETE /settings/providers/{id}", a.deleteProvider)
	a.Mux.HandleFunc("POST /authorize", a.authorize)
	a.Mux.HandleFunc("GET /callback/{provider}", a.callback)
	a.Mux.HandleFunc("POST /complete-binding", a.completeBinding)
	a.Mux.HandleFunc("POST /registration", a.registration)
	a.Mux.HandleFunc("POST /complete-registration", a.completeRegistration)
	a.Mux.HandleFunc("GET /bindings", a.bindings)
	a.Mux.HandleFunc("DELETE /bindings/{id}", a.deleteBinding)
	a.Mux.HandleFunc("POST /bindings/import", a.importBindings)
	a.View("providers", a.providerView)
	return a
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func localPath(path string) string {
	u, e := url.Parse(path)
	if e != nil || u.IsAbs() || u.Host != "" || strings.ContainsAny(path, "\\\r\n") {
		return "/user"
	}
	if u.Path == "/user" || strings.HasPrefix(u.Path, "/user/") || u.Path == "/account" || strings.HasPrefix(u.Path, "/account/") {
		return path
	}
	return "/user"
}

func expiresAfter(duration time.Duration) string {
	return time.Now().Add(duration).UTC().Format(time.RFC3339)
}

func (a *app) cleanup(collection string) error {
	rows, e := a.C.List(collection)
	if e != nil {
		return e
	}
	for _, row := range rows {
		if p.Text(row, "expiresAt") < c.Now() {
			if e = a.C.Delete(collection, p.Text(row, "id")); e != nil {
				return e
			}
		}
	}
	return nil
}
