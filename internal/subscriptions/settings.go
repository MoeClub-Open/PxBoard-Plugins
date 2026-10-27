package subscriptions

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"strings"
)

func (a *app) subscriptionSettings() (p.Record, error) {
	rows, err := a.C.List("settings")
	if err != nil {
		return nil, err
	}
	config := p.Record{"id": "settings", "hideFromBrowsers": true, "browserAgents": "Mozilla/"}
	for _, row := range rows {
		if p.Text(row, "id") == "settings" {
			for _, key := range []string{"hideFromBrowsers", "browserAgents"} {
				if value, ok := row[key]; ok {
					config[key] = value
				}
			}
			break
		}
	}
	return config, nil
}

func (a *app) publicSettings(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	config, err := a.subscriptionSettings()
	if !c.Err(w, err) {
		p.JSON(w, p.Record{"hideFromBrowsers": p.Bool(config, "hideFromBrowsers")})
	}
}

func (a *app) settings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	config, err := a.subscriptionSettings()
	if c.Err(w, err) {
		return
	}
	if r.Method == http.MethodGet {
		p.JSON(w, []p.Record{config})
		return
	}
	var body struct {
		HideFromBrowsers *bool   `json:"hideFromBrowsers"`
		BrowserAgents    *string `json:"browserAgents"`
	}
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	if body.HideFromBrowsers != nil {
		config["hideFromBrowsers"] = *body.HideFromBrowsers
	}
	if body.BrowserAgents != nil {
		config["browserAgents"] = *body.BrowserAgents
	}
	if strings.TrimSpace(p.Text(config, "browserAgents")) == "" {
		p.Fail(w, 400, "请至少填写一个浏览器 User-Agent 关键词")
		return
	}
	if !c.Err(w, a.C.Put("settings", config)) {
		p.JSON(w, config)
	}
}
