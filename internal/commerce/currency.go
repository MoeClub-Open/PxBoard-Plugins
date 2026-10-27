package commerce

import (
	"fmt"
	"net/http"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func currency(config p.Record) p.Record {
	name, symbol := p.Text(config, "currencyName"), p.Text(config, "currencySymbol")
	if name == "" {
		name = "元"
	}
	if symbol == "" {
		symbol = "¥"
	}
	return p.Record{"currencyName": name, "currencySymbol": symbol}
}

func (a *app) currencySettings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	config, err := a.config()
	if c.Err(w, err) {
		return
	}
	if r.Method == "GET" {
		p.JSON(w, []p.Record{currency(config)})
		return
	}
	var body struct {
		CurrencyName   string `json:"currencyName"`
		CurrencySymbol string `json:"currencySymbol"`
	}
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	body.CurrencyName, body.CurrencySymbol = strings.TrimSpace(body.CurrencyName), strings.TrimSpace(body.CurrencySymbol)
	if body.CurrencyName == "" || body.CurrencySymbol == "" {
		p.Fail(w, 400, "请填写金额单位和符号")
		return
	}
	config["currencyName"], config["currencySymbol"] = body.CurrencyName, body.CurrencySymbol
	if !c.Err(w, a.C.Put("settings", config)) {
		p.JSON(w, currency(config))
	}
}
func (a *app) publicCurrency(w http.ResponseWriter, r *http.Request) {
	config, err := a.config()
	if c.Err(w, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, currency(config))
}
func (a *app) currencyView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可修改金额单位")
	}
	fields := []p.Field{c.Field("currencyName", "金额单位", "text", true), c.Field("currencySymbol", "金额符号", "text", true)}
	fields[0].Hint = "仅改变显示，不会换算金额或更改支付渠道的结算币种"
	return p.View{Title: "金额设置", Resources: []p.Resource{{Key: "currency", Title: "金额单位", Endpoint: "/currency", Singleton: true, Editable: true, Fields: fields, Columns: fields}}}, nil
}
