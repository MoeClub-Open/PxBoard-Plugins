package branding

import (
	"bytes"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"html/template"
	"net/http"
	"strings"
)

type landingContent struct {
	Name, LogoURL, Heading, Introduction, Footer string
	RegistrationOpen                             bool
	Highlights                                   []string
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
	content := landingContent{Name: name, LogoURL: p.Text(config, "logoUrl"), Heading: heading, Introduction: p.Text(config, "introduction"), Footer: p.Text(config, "footer"), RegistrationOpen: context.RegistrationOpen, Highlights: []string{}}
	for _, line := range strings.Split(p.Text(config, "highlights"), "\n") {
		if text := strings.TrimSpace(line); text != "" {
			content.Highlights = append(content.Highlights, text)
		}
	}
	description := p.Text(config, "description")
	if description == "" {
		description = content.Introduction
	}
	if description == "" {
		description = name + "的账户与订阅服务。"
	}
	image := p.Text(config, "socialImageUrl")
	if strings.HasPrefix(image, "/") {
		if context.BaseURL != "" {
			image = context.BaseURL + image
		} else {
			image = ""
		}
	}
	result := p.PublicSite{Name: name, Title: title, Description: description, LogoURL: content.LogoURL, FaviconURL: p.Text(config, "faviconUrl"), ImageURL: image, Renderer: "storefront", Indexable: p.Bool(config, "indexingEnabled")}
	if context.Content {
		var html bytes.Buffer
		if c.Err(w, landingTemplate.Execute(&html, content)) {
			return
		}
		result.HTML = html.String()
		result.Data = p.Record{"heading": heading, "introduction": content.Introduction, "highlights": content.Highlights, "footer": content.Footer}
	}
	p.JSON(w, result)
}

var landingTemplate = template.Must(template.New("landing").Parse(`<div class="storefront">
<header class="storefront-header"><div class="storefront-header-inner"><a href="/" class="storefront-name">{{if .LogoURL}}<img class="brand-logo" src="{{.LogoURL}}" alt="">{{end}}{{.Name}}</a>{{if .Highlights}}<nav aria-label="首页导航"><a class="storefront-nav-link" href="#services">服务说明</a></nav>{{end}}</div></header>
<main class="storefront-main"><section class="storefront-hero"><div class="storefront-copy"><p class="storefront-eyebrow">网络与订阅服务</p><h1>{{.Heading}}</h1>{{if .Introduction}}<p class="storefront-intro-text">{{.Introduction}}</p>{{end}}<div class="storefront-actions"><a class="storefront-action storefront-primary" href="/login">登录账户 <span aria-hidden="true">→</span></a>{{if .RegistrationOpen}}<a class="storefront-action storefront-secondary" href="/register">注册账户</a>{{end}}</div></div><div class="storefront-art" aria-hidden="true"></div></section>
{{if .Highlights}}<section id="services" class="storefront-highlights" aria-labelledby="storefront-services-heading"><h2 id="storefront-services-heading">服务说明</h2><ul>{{range .Highlights}}<li>{{.}}</li>{{end}}</ul></section>{{end}}</main>
<footer class="storefront-footer"><span>© {{.Name}}</span>{{if .Footer}}<p>{{.Footer}}</p>{{end}}</footer></div>`))
