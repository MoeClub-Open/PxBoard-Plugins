package commerce

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"math"
	"net/http"
)

func (a *app) config() (p.Record, error) {
	rows, e := a.C.List("settings")
	if e != nil {
		return nil, e
	}
	out := p.Record{"id": "settings", "registrationEnabled": true, "commissionPercent": 10.0, "commissionHoldDays": 7.0, "commissionFirstPurchaseOnly": true}
	if len(rows) > 0 {
		for k, v := range rows[0] {
			out[k] = v
		}
	}
	return out, nil
}
func (a *app) settings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	if r.Method == "GET" {
		p.JSON(w, []p.Record{config})
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	for _, key := range []string{"registrationEnabled", "commissionPercent", "commissionHoldDays", "commissionFirstPurchaseOnly"} {
		if v, ok := b[key]; ok {
			config[key] = v
		}
	}
	if p.Number(config, "commissionPercent") < 0 || p.Number(config, "commissionPercent") > 100 || p.Number(config, "commissionHoldDays") < 0 || math.Trunc(p.Number(config, "commissionHoldDays")) != p.Number(config, "commissionHoldDays") {
		p.Fail(w, 400, "返佣比例或冻结天数无效")
		return
	}
	if !c.Err(w, a.C.Put("settings", config)) {
		p.JSON(w, true)
	}
}
func (a *app) settingsView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("无权修改运营设置")
	}
	return p.View{Title: "运营设置", Resources: []p.Resource{{Key: "settings", Title: "运营规则", Endpoint: "/settings", Editable: true, Columns: []p.Field{c.Field("registrationEnabled", "开放注册", "boolean", false), c.Field("commissionPercent", "返佣比例（%）", "number", false), c.Field("commissionHoldDays", "佣金冻结（天）", "number", false), c.Field("commissionFirstPurchaseOnly", "仅首次购买返佣", "boolean", false)}, Fields: []p.Field{c.Field("registrationEnabled", "开放注册", "boolean", false), c.Field("commissionPercent", "返佣比例（%）", "number", true), c.Field("commissionHoldDays", "佣金冻结（天）", "number", true), c.Field("commissionFirstPurchaseOnly", "仅首次购买返佣", "boolean", false)}}}}, nil
}
