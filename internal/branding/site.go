package branding

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type landingContent struct {
	Name, LogoURL, Heading, Introduction, Footer string
	RegistrationOpen                             bool
	Routes                                       []routeEntry
	Features                                     []featureEntry
}

func (a *app) site(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "core") {
		return
	}
	var context p.SiteContext
	if c.Err(w, p.Decode(r, &context)) {
		return
	}
	config, err := a.config()
	if c.Err(w, err) {
		return
	}
	name := p.Text(config, "name")
	if name == "" {
		name = context.Name
	}
	heading := p.Text(config, "heading")
	if heading == "" {
		heading = name
	}
	title := p.Text(config, "seoTitle")
	if title == "" {
		title = heading
	}
	routes, _ := parseRoutes(config["routes"])
	features, _ := parseFeatures(config["features"])
	content := landingContent{
		Name:             name,
		LogoURL:          p.Text(config, "logoUrl"),
		Heading:          heading,
		Introduction:     strings.TrimSpace(p.Text(config, "introduction")),
		Footer:           p.Text(config, "footer"),
		RegistrationOpen: context.RegistrationOpen,
		Routes:           routes,
		Features:         features,
	}
	description := p.Text(config, "description")
	if description == "" {
		description = content.Introduction
	}
	if description == "" {
		description = name + "的加密网络接入与订阅管理服务。"
	}
	image := p.Text(config, "socialImageUrl")
	if strings.HasPrefix(image, "/") {
		if context.BaseURL != "" {
			image = context.BaseURL + image
		} else {
			image = ""
		}
	}
	result := p.PublicSite{
		Name:        name,
		Title:       title,
		Description: description,
		LogoURL:     content.LogoURL,
		FaviconURL:  p.Text(config, "faviconUrl"),
		ImageURL:    image,
		Renderer:    "storefront",
		Indexable:   p.Bool(config, "indexingEnabled"),
	}
	if context.Content {
		var html bytes.Buffer
		if c.Err(w, landingTemplate.Execute(&html, content)) {
			return
		}
		result.HTML = html.String()
		result.Data = p.Record{
			"heading":      heading,
			"introduction": content.Introduction,
			"routes":       content.Routes,
			"features":     content.Features,
			"footer":       content.Footer,
		}
	}
	p.JSON(w, result)
}

var landingTemplate = template.Must(template.New("landing").Parse(`<div class="storefront">
<header class="storefront-header"><div class="storefront-header-inner"><a href="/" class="storefront-name"><div class="brand">{{if .LogoURL}}<img class="brand-logo" src="{{.LogoURL}}" alt="">{{end}}<span>{{.Name}}</span></div></a><nav aria-label="首页导航">{{if .Routes}}<a class="storefront-nav-link" href="#routes">线路与网络</a>{{end}}{{if .Features}}<a class="storefront-nav-link" href="#features">功能特性</a>{{end}}</nav></div></header>
<main class="storefront-main"><section class="storefront-hero"><div class="storefront-container"><div class="storefront-hero-inner"><h1>{{.Heading}}</h1>{{if .Introduction}}<p class="storefront-intro-text">{{.Introduction}}</p>{{end}}<div class="storefront-actions"><a class="storefront-action" href="/login">登录账户</a>{{if .RegistrationOpen}}<a class="storefront-action" href="/register">注册账户</a>{{end}}</div></div></div></section>
{{if .Routes}}<section id="routes" class="storefront-section" aria-labelledby="storefront-routes-heading"><div class="storefront-container"><div class="storefront-section-head"><h2 id="storefront-routes-heading">线路与网络</h2></div><div class="storefront-routes-grid">{{range .Routes}}<article class="storefront-route-card">{{if .Region}}<span class="route-tag">{{.Region}}</span>{{end}}<h3>{{.Title}}</h3>{{if .Description}}<p>{{.Description}}</p>{{end}}</article>{{end}}</div></div></section>{{end}}
{{if .Features}}<section id="features" class="storefront-section" aria-labelledby="storefront-features-heading"><div class="storefront-container"><div class="storefront-section-head"><h2 id="storefront-features-heading">功能特性</h2></div><div class="storefront-features-grid">{{range .Features}}<article class="storefront-feature-card"><h3>{{.Title}}</h3>{{if .Description}}<p>{{.Description}}</p>{{end}}</article>{{end}}</div></div></section>{{end}}</main>
<footer class="storefront-footer"><div class="storefront-footer-inner"><span>© {{.Name}}</span>{{if .Footer}}<p>{{.Footer}}</p>{{end}}</div></footer></div>`))
