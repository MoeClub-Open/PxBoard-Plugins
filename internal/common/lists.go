package common

import (
	"net/http"
	"strconv"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func OwnerFilters(r *http.Request) []p.Record {
	if p.Admin(r) && r.URL.Query().Get("mine") != "1" {
		return nil
	}
	return []p.Record{{"field": "userId", "value": p.Subject(r).ID}}
}

func (a *App) ReadRecords(collection string, r *http.Request, filters []p.Record, searchFields ...string) (p.QueryResult, error) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		rows, err := a.QueryAll(collection, filters...)
		return p.QueryResult{Items: rows, Total: len(rows)}, err
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	query := p.Query{Collection: collection, Limit: limit, Offset: offset, Search: r.URL.Query().Get("q"), SearchFields: searchFields}
	for _, filter := range filters {
		query.Filters = append(query.Filters, p.Filter{Field: p.Text(filter, "field"), Value: filter["value"], Op: p.Text(filter, "op")})
	}
	return a.C.Query(query)
}

func ListResponse(w http.ResponseWriter, r *http.Request, result p.QueryResult) {
	if r.URL.Query().Get("limit") != "" {
		p.JSON(w, result)
	} else {
		p.JSON(w, result.Items)
	}
}
