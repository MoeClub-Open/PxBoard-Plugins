package access

import (
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"time"
)

func cycleNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func subjectUser(r *http.Request) string {
	if p.Admin(r) && r.URL.Query().Get("mine") != "1" {
		return ""
	}
	return p.Subject(r).ID
}

func nextNumericID(rows []p.Record) float64 {
	number := 0.0
	for _, row := range rows {
		number = max(number, p.Number(row, "numericId"))
	}
	return number + 1
}

func (a *app) recordsFor(collection, field, value string) ([]p.Record, error) {
	var result struct {
		Items []p.Record `json:"items"`
	}
	e := a.C.Request("POST", "/api/v1/runtime/query", p.Record{
		"collection": collection, "filters": []p.Record{{"field": field, "value": value}}, "limit": 1000,
	}, &result)
	return result.Items, e
}

func (a *app) enrichedFor(user string) ([]p.Record, error) {
	var rows []p.Record
	var e error
	if user == "" {
		rows, e = a.C.List("accounts")
	} else {
		rows, e = a.recordsFor("accounts", "userId", user)
	}
	if e != nil {
		return nil, e
	}
	var usage []p.Record
	if user == "" {
		ids := []string{}
		for _, row := range rows {
			ids = append(ids, p.Text(row, "userId")+"-"+p.Text(row, "cycleStart"))
		}
		for offset := 0; offset < len(ids); offset += 500 {
			var current []p.Record
			current, e = a.QueryAll("usage", p.Record{"field": "id", "op": "in", "value": ids[offset:min(offset+500, len(ids))]})
			if e != nil {
				return nil, e
			}
			usage = append(usage, current...)
		}
	} else if len(rows) > 0 {
		usage, e = a.recordsFor("usage", "id", user+"-"+p.Text(rows[0], "cycleStart"))
	}
	if e != nil {
		return nil, e
	}
	users, e := a.UsersFor(rows, "userId")
	if e != nil {
		return nil, e
	}
	byUser := make(map[string]p.Record, len(users))
	for _, row := range users {
		byUser[p.Text(row, "id")] = row
	}
	totals := make(map[string]float64, len(usage))
	for _, row := range usage {
		totals[p.Text(row, "id")] = p.Number(row, "bytes")
	}
	for _, row := range rows {
		if p.Number(row, "numericId") == 0 {
			all, err := a.C.List("accounts")
			if err != nil {
				return nil, err
			}
			row["numericId"] = nextNumericID(all)
			if e = a.C.Put("accounts", row); e != nil {
				return nil, e
			}
		}
		used := totals[p.Text(row, "userId")+"-"+p.Text(row, "cycleStart")] / 1073741824
		row["usedGiB"] = used
		u := byUser[p.Text(row, "userId")]
		row["email"] = u["email"]
		expires, err := time.Parse(time.RFC3339, p.Text(row, "expiresAt"))
		validUntil := p.Text(row, "expiresAt") == "" || err == nil && expires.After(time.Now())
		row["effective"] = p.Text(u, "status") == "active" && p.Text(row, "status") == "active" && (p.Bool(row, "unlimited") || used < p.Number(row, "quotaGiB")) && validUntil
	}
	return rows, nil
}

func clientAccount(row p.Record) p.Record {
	out := p.Record{}
	for _, key := range []string{"id", "userId", "numericId", "credential", "proxyPassword", "email", "effective", "status", "quotaGiB", "unlimited", "usedGiB", "expiresAt", "groups", "accessLevel", "levelExpiresAt", "allNodeGroups", "speedLimitMbps", "deviceLimit", "planFamilyId", "cycleStart", "entitlementRevision", "lastGrantId"} {
		if value, ok := row[key]; ok {
			out[key] = value
		}
	}
	return out
}

func (a *app) userAccount(w http.ResponseWriter, r *http.Request) {
	if !p.Admin(r) && !p.Service(r, "commerce") && !p.Service(r, "nodes") && !p.Service(r, "redeem") {
		p.Fail(w, 403, "无权读取账户权益")
		return
	}
	rows, e := a.enrichedFor(r.PathValue("user"))
	if e != nil {
		p.Fail(w, 400, e.Error())
		return
	}
	if len(rows) == 0 {
		p.JSON(w, nil)
		return
	}
	out := clientAccount(rows[0])
	if p.Service(r, "commerce") || p.Admin(r) {
		out["planSourceId"] = rows[0]["planSourceId"]
	}
	p.JSON(w, out)
}
