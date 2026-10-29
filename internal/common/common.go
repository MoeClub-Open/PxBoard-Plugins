package common

import (
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"slices"
	"strings"
	"time"
)

type App struct {
	C          *p.Client
	Mux        *http.ServeMux
	Concurrent []string
}

func (a *App) ConcurrentPaths() []string { return a.Concurrent }

func (a *App) Summary(fn func(*http.Request) ([]p.Metric, error)) {
	a.Mux.HandleFunc("GET /summary", func(w http.ResponseWriter, r *http.Request) {
		if !Member(w, r) {
			return
		}
		metrics, e := fn(r)
		if !Err(w, e) {
			p.JSON(w, metrics)
		}
	})
}

func New() *App                                                 { return &App{C: p.NewClient(), Mux: http.NewServeMux()} }
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.Mux.ServeHTTP(w, r) }
func Err(w http.ResponseWriter, e error) bool {
	if e != nil {
		p.Fail(w, 400, e.Error())
		return true
	}
	return false
}
func Require(w http.ResponseWriter, r *http.Request, services ...string) bool {
	if p.Admin(r) {
		return true
	}
	for _, id := range services {
		if p.Service(r, id) {
			return true
		}
	}
	p.Fail(w, 403, "无权执行此操作")
	return false
}
func Member(w http.ResponseWriter, r *http.Request) bool {
	a := p.Subject(r)
	if a.ID == "" || a.Role == "guest" {
		p.Fail(w, 401, "请先登录")
		return false
	}
	return true
}
func (a *App) Find(collection, id string) (p.Record, error) {
	return a.C.Get(collection, id)
}
func (a *App) Filter(collection string, keep func(p.Record) bool) ([]p.Record, error) {
	rows, e := a.C.List(collection)
	if e != nil {
		return nil, e
	}
	out := []p.Record{}
	for _, r := range rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// QueryAll reads every page using the same filtering and pagination contract.
func (a *App) QueryAll(collection string, filters ...p.Record) ([]p.Record, error) {
	rows := []p.Record{}
	for {
		var page struct {
			Items []p.Record `json:"items"`
			Total int        `json:"total"`
		}
		if err := a.C.Request("POST", "/api/v1/runtime/query", p.Record{"collection": collection, "filters": filters, "offset": len(rows), "limit": 1000}, &page); err != nil {
			return nil, err
		}
		rows = append(rows, page.Items...)
		if len(rows) >= page.Total || len(page.Items) == 0 {
			return rows, nil
		}
	}
}
func (a *App) CRUD(resource p.Resource, normalize func(*http.Request, p.Record, p.Record) error, read ...func(*http.Request, p.Record) bool) {
	path := resource.Endpoint
	collection := resource.Key
	a.Mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		if len(read) > 0 {
			if !Member(w, r) {
				return
			}
		} else if !Require(w, r) {
			return
		}
		rows, e := a.C.List(collection)
		if !Err(w, e) {
			if len(read) > 0 {
				filtered := []p.Record{}
				for _, row := range rows {
					if read[0](r, row) {
						filtered = append(filtered, row)
					}
				}
				rows = filtered
			}
			p.JSON(w, rows)
		}
	})
	if resource.CreateLabel != "" {
		a.Mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			if !Require(w, r) {
				return
			}
			var b p.Record
			if Err(w, p.Decode(r, &b)) {
				return
			}
			delete(b, "id")
			if normalize != nil && Err(w, normalize(r, b, nil)) {
				return
			}
			b["createdAt"] = Now()
			if !Err(w, a.C.Put(collection, b)) {
				p.JSON(w, b)
			}
		})
	}
	if resource.Editable {
		a.Mux.HandleFunc("PATCH "+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			if !Require(w, r) {
				return
			}
			old, e := a.Find(collection, r.PathValue("id"))
			if Err(w, e) {
				return
			}
			var b p.Record
			if Err(w, p.Decode(r, &b)) {
				return
			}
			b["id"] = old["id"]
			if normalize != nil && Err(w, normalize(r, b, old)) {
				return
			}
			for k, v := range b {
				old[k] = v
			}
			if !Err(w, a.C.Put(collection, old)) {
				p.JSON(w, old)
			}
		})
	}
	if resource.Deletable {
		a.Mux.HandleFunc("DELETE "+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			if !Require(w, r) {
				return
			}
			if !Err(w, a.C.Delete(collection, r.PathValue("id"))) {
				p.JSON(w, true)
			}
		})
	}
}
func (a *App) View(id string, fn func(*http.Request) (p.View, error)) {
	a.Mux.HandleFunc("GET /ui/"+id, func(w http.ResponseWriter, r *http.Request) {
		if !Member(w, r) {
			return
		}
		v, e := fn(r)
		if !Err(w, e) {
			p.JSON(w, v)
		}
	})
}
func (a *App) Events(fn func(p.Event) error) {
	a.Mux.HandleFunc("POST /_events", func(w http.ResponseWriter, r *http.Request) {
		if !p.Service(r, "core") {
			p.Fail(w, 403, "无权处理内部事件")
			return
		}
		var e p.Event
		if Err(w, p.Decode(r, &e)) {
			return
		}
		if !Err(w, fn(e)) {
			p.JSON(w, true)
		}
	})
}
func Field(key, label, kind string, required bool) p.Field {
	return p.Field{Key: key, Label: label, Type: kind, Required: required}
}
func GroupField(key, label string, required bool) p.Field {
	return p.Field{Key: key, Label: label, Type: "groups", Required: required, OptionsEndpoint: "/api/v1/extensions/nodes/groups/options"}
}
func Select(key, label string, options ...string) p.Field {
	f := Field(key, label, "select", true)
	for _, o := range options {
		parts := strings.SplitN(o, "|", 2)
		l := o
		if len(parts) == 2 {
			l = parts[1]
		}
		f.Options = append(f.Options, p.Option{Label: l, Value: parts[0]})
	}
	return f
}
func UserField() p.Field {
	return p.Field{Key: "userId", Label: "用户", Type: "select", Required: true, OptionsEndpoint: "/api/v1/users/options", OptionsSearch: true}
}

func (a *App) UsersFor(rows []p.Record, field string) ([]p.Record, error) {
	seen := map[string]bool{}
	ids := []string{}
	for _, row := range rows {
		id := p.Text(row, field)
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return a.C.UsersByID(ids...)
}
func Own(r *http.Request, row p.Record) bool {
	return p.Admin(r) || p.Text(row, "userId") == p.Subject(r).ID
}
func Now() string { return time.Now().UTC().Format(time.RFC3339) }
func Defaults(b, old p.Record, values p.Record) {
	for k, v := range values {
		if _, ok := b[k]; !ok {
			if old != nil && old[k] != nil {
				b[k] = old[k]
			} else {
				b[k] = v
			}
		}
	}
}
func Validate(b p.Record, fields ...string) error {
	for _, k := range fields {
		if strings.TrimSpace(p.Text(b, k)) == "" {
			return fmt.Errorf("请填写 %s", k)
		}
	}
	return nil
}
func Allowed(value string, values ...string) bool { return slices.Contains(values, value) }
