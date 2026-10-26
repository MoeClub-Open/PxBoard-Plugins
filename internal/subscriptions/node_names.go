package subscriptions

import (
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"sort"
)

func sortedNodes(nodes []p.Record) []p.Record {
	sort.SliceStable(nodes, func(i, j int) bool {
		if p.Number(nodes[i], "sort") != p.Number(nodes[j], "sort") {
			return p.Number(nodes[i], "sort") < p.Number(nodes[j], "sort")
		}
		return p.Text(nodes[i], "id") < p.Text(nodes[j], "id")
	})
	used := map[string]bool{"代理选择": true, "DIRECT": true, "proxy": true, "direct": true}
	for _, node := range nodes {
		name := p.Text(node, "name")
		if used[name] {
			name = fmt.Sprintf("%s · %.0f", name, p.Number(node, "wireId"))
			for suffix := 2; used[name]; suffix++ {
				name = fmt.Sprintf("%s · %d", p.Text(node, "name"), suffix)
			}
			node["name"] = name
		}
		used[name] = true
	}
	return nodes
}
