package branding

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type landingPeriod struct{ ID, Label, Price, URL string }
type landingPlan struct {
	Name, Description, Quota, URL string
	Periods                       []landingPeriod
}
type landingContent struct {
	Name, LogoURL, Heading, Introduction, Footer string
	RegistrationOpen, CommerceAvailable          bool
	Highlights                                   []string
	Plans                                        []landingPlan
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
	content := landingContent{Name: name, LogoURL: p.Text(config, "logoUrl"), Heading: heading, Introduction: p.Text(config, "introduction"), Footer: p.Text(config, "footer"), RegistrationOpen: context.RegistrationOpen, Plans: []landingPlan{}, Highlights: []string{}}
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
	if !context.Content {
		p.JSON(w, p.PublicSite{Name: name, Title: title, Description: description, LogoURL: content.LogoURL, FaviconURL: p.Text(config, "faviconUrl"), Indexable: p.Bool(config, "indexingEnabled")})
		return
	}
	var plans []p.Record
	err = a.C.Call("commerce", "GET", "/public/plans", nil, &plans)
	content.CommerceAvailable = err == nil
	currency := p.Record{"currencySymbol": "¥", "currencyName": "元"}
	if content.CommerceAvailable {
		_ = a.C.Call("commerce", "GET", "/public/currency", nil, &currency)
		for _, plan := range plans {
			item := landingPlan{Name: p.Text(plan, "name"), Description: p.Text(plan, "description"), Periods: []landingPeriod{}}
			if p.Text(plan, "kind") == "time" {
				item.Quota = "维持原额度"
			} else if p.Bool(plan, "unlimited") {
				item.Quota = "不限流量"
			} else {
				item.Quota = fmt.Sprintf("%g GB / 周期", p.Number(plan, "quotaGiB"))
			}
			periods, _ := plan["periods"].([]any)
			for _, raw := range periods {
				period, _ := raw.(map[string]any)
				if kind := p.Text(period, "kind"); kind != "subscription" && kind != "time" {
					continue
				}
				path := "/user/plans?plan=" + url.QueryEscape(p.Text(plan, "id"))
				item.URL = "/login?redirect=" + url.QueryEscape(path)
				item.Periods = append(item.Periods, landingPeriod{ID: p.Text(period, "id"), Label: p.Text(period, "label"), Price: fmt.Sprintf("%s %.2f", p.Text(currency, "currencySymbol"), p.Number(period, "price")/100), URL: "/login?redirect=" + url.QueryEscape(path)})
			}
			if len(item.Periods) > 0 {
				content.Plans = append(content.Plans, item)
			}
		}
	}
	var html bytes.Buffer
	if c.Err(w, landingTemplate.Execute(&html, content)) {
		return
	}
	image := p.Text(config, "socialImageUrl")
	if strings.HasPrefix(image, "/") {
		if context.BaseURL != "" {
			image = context.BaseURL + image
		} else {
			image = ""
		}
	}
	p.JSON(w, p.PublicSite{Name: name, Title: title, Description: description, LogoURL: content.LogoURL, FaviconURL: p.Text(config, "faviconUrl"), ImageURL: image, Renderer: "storefront", Indexable: p.Bool(config, "indexingEnabled"), HTML: html.String(), Data: p.Record{"heading": heading, "introduction": content.Introduction, "highlights": content.Highlights, "footer": content.Footer, "commerceAvailable": content.CommerceAvailable, "plans": plans, "currency": currency}})
}

var landingTemplate = template.Must(template.New("landing").Parse(`<div class="storefront">
<header class="storefront-header"><div class="storefront-header-inner"><a href="/" class="storefront-name">{{if .LogoURL}}<img class="brand-logo" src="{{.LogoURL}}" alt="">{{end}}{{.Name}}</a><nav aria-label="账户"><a class="storefront-link" href="/login">登录</a>{{if .RegistrationOpen}}<a class="storefront-link storefront-primary" href="/register">注册</a>{{end}}</nav></div></header>
<main class="storefront-main"><section class="storefront-introduction"><h1>{{.Heading}}</h1>{{if .Introduction}}<p class="storefront-intro-text">{{.Introduction}}</p>{{end}}{{if .CommerceAvailable}}<a class="storefront-link storefront-primary" href="#plans">查看套餐</a>{{else}}<a class="storefront-link storefront-primary" href="/login">登录账户</a>{{end}}</section>
{{if .Highlights}}<section class="storefront-highlights" aria-label="服务说明"><ul>{{range .Highlights}}<li>{{.}}</li>{{end}}</ul></section>{{end}}
{{if .CommerceAvailable}}<section id="plans"><h2>服务套餐</h2>{{if .Plans}}<div class="storefront-table"><table><thead><tr><th>套餐</th><th>流量</th><th>购买选项</th><th>购买</th></tr></thead><tbody>{{range .Plans}}<tr><td><strong>{{.Name}}</strong>{{if .Description}}<p>{{.Description}}</p>{{end}}</td><td>{{.Quota}}</td><td>{{range .Periods}}<p><span>{{.Label}} · {{.Price}}</span></p>{{end}}</td><td><a class="storefront-link" href="{{.URL}}">选择</a></td></tr>{{end}}</tbody></table></div>{{else}}<p class="storefront-empty">暂未开放套餐购买。</p>{{end}}</section>{{end}}</main>
<footer class="storefront-footer">{{if .Footer}}<p>{{.Footer}}</p>{{end}}<span>{{.Name}}</span></footer></div>`))
