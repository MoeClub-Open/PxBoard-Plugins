package common

import (
	"strings"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *App) SiteName() string {
	var branding p.Record
	if a.C.Call("branding", "GET", "/public", nil, &branding) == nil {
		if name := strings.TrimSpace(p.Text(branding, "name")); name != "" {
			return name
		}
	}
	return "PxBoard"
}
