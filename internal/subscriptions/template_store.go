package subscriptions

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func templateKey(format string) string {
	if format == "sing-box" {
		return "singboxTemplateId"
	}
	return "clashTemplateId"
}

func initialTemplateSettings() p.Record {
	return p.Record{"id": "templates", "clashTemplateId": "builtin-clash", "singboxTemplateId": "builtin-sing-box", "profileTitle": "", "homepage": "", "updateIntervalHours": 6}
}

func (a *app) templateSettings() (p.Record, error) {
	config, err := a.C.Get("settings", "templates")
	if err == nil {
		return config, nil
	}
	var apiError *p.APIError
	if !errors.As(err, &apiError) || apiError.Status != 404 {
		return nil, err
	}
	config = initialTemplateSettings()
	writes := []p.Write{{Collection: "settings", Record: config}}
	for _, format := range []string{"clash", "sing-box"} {
		name := "Mihomo 默认分流"
		if format == "sing-box" {
			name = "sing-box 默认分流"
		}
		writes = append(writes, p.Write{Collection: "templates", Record: p.Record{"id": "builtin-" + format, "name": name, "format": format, "content": sc.Default(format), "enabled": true, "createdAt": c.Now()}})
	}
	if err := a.C.Commit("templates:initialize", writes); err != nil {
		return nil, err
	}
	return a.C.Get("settings", "templates")
}

func (a *app) selectedTemplate(format string) (p.Record, p.Record, error) {
	settings, err := a.templateSettings()
	if err != nil {
		return nil, nil, err
	}
	template, err := a.C.Get("templates", p.Text(settings, templateKey(format)))
	if err != nil || !p.Bool(template, "enabled") || p.Text(template, "format") != format {
		return nil, nil, fmt.Errorf("默认订阅模板不可用，请管理员检查模板设置")
	}
	return template, settings, nil
}

func (a *app) profileTitle(settings p.Record) string {
	if title := strings.TrimSpace(p.Text(settings, "profileTitle")); title != "" {
		return title
	}
	var branding p.Record
	if a.C.Call("branding", "GET", "/public", nil, &branding) == nil && strings.TrimSpace(p.Text(branding, "name")) != "" {
		return p.Text(branding, "name")
	}
	return "PxBoard"
}

func (a *app) directHosts(host, homepage string) ([]string, error) {
	values := []string{host, homepage}
	routes, err := a.C.List("public_routes")
	if err != nil {
		return nil, err
	}
	for _, route := range routes {
		if p.Bool(route, "enabled") {
			values = append(values, strings.FieldsFunc(p.Text(route, "baseUrls"), func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })...)
		}
	}
	return values, nil
}

func validHomepage(value string) bool {
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && !strings.ContainsAny(value, "\r\n")
}
