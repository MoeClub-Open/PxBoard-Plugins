package migration

import (
	"fmt"
	"sort"
	"strings"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (m *mapper) v2Business() {
	if m.Options.IncludeHistory {
		m.v2Orders()
		m.v2Coupons()
	}
	if m.Options.IncludeSupport {
		m.v2Support()
	}
	if m.Options.IncludeRedeem {
		m.v2Redeem()
	}
}

func (m *mapper) v2Orders() {
	plans := map[string]Row{}
	for _, row := range m.S.Rows("v2_plan") {
		plans[text(row, "id")] = row
	}
	table, rows := m.table("v2_order")
	states := map[int64]string{0: "canceled", 1: "paid", 2: "canceled", 3: "fulfilled", 4: "credited"}
	for _, row := range rows {
		user := m.userID(row, "user_id")
		if user == "" {
			continue
		}
		state, ok := states[integer(row, "status")]
		if !ok {
			m.fail("订单 %s 的状态无法识别", text(row, "id"))
			continue
		}
		period := text(row, "period")
		for old, current := range legacyPeriods {
			if period == old {
				period = current
			}
		}
		name := text(plans[text(row, "plan_id")], "name")
		if name == "" {
			name = "套餐 " + text(row, "plan_id")
		}
		if definition, exists := periods[period]; exists {
			name += " · " + definition.label
		}
		number := text(row, "trade_no")
		if number == "" {
			number = m.id("order", text(row, "id"))
		}
		method := "online"
		if integer(row, "payment_id") == 0 {
			method = "wallet"
		}
		order := p.Record{"number": number, "userId": user, "planName": name, "kind": "subscription", "periodId": period, "amount": m.money(row, "total_amount", false), "balanceAmount": m.money(row, "balance_amount", false), "discount": m.money(row, "discount_amount", false), "refundAmount": m.money(row, "refund_amount", false), "currency": "CNY", "status": state, "imported": true, "method": method, "createdAt": m.created(row, "created_at"), "updatedAt": m.created(row, "updated_at", "created_at"), "paidAt": m.timeValue(row, "paid_at"), "source": m.ref(row, table), "sourceContent": row}
		if coupon := text(row, "coupon_id"); coupon != "" && coupon != "0" {
			order["couponId"] = m.id("coupon", coupon)
		}
		m.doc("commerce", "orders", m.id("order", text(row, "id")), order)
	}
}

func (m *mapper) v2Support() {
	users := map[string]Row{}
	for _, row := range m.S.Rows("v2_user") {
		users[text(row, "id")] = row
	}
	_, messages := m.table("v2_ticket_message")
	threads := map[string][]Row{}
	for _, row := range messages {
		threads[text(row, "ticket_id")] = append(threads[text(row, "ticket_id")], row)
	}
	table, tickets := m.table("v2_ticket")
	for _, ticket := range tickets {
		user := m.userID(ticket, "user_id")
		if user == "" {
			continue
		}
		items := threads[text(ticket, "id")]
		sort.SliceStable(items, func(i, j int) bool {
			if integer(items[i], "created_at") == integer(items[j], "created_at") {
				return integer(items[i], "id") < integer(items[j], "id")
			}
			return integer(items[i], "created_at") < integer(items[j], "created_at")
		})
		thread := []p.Record{}
		for _, message := range items {
			author := users[text(message, "user_id")]
			admin := truth(author, "is_admin") || truth(author, "is_staff")
			email := text(author, "email")
			if email == "" {
				email = "客服"
				admin = text(message, "user_id") != text(ticket, "user_id")
			}
			thread = append(thread, p.Record{"id": m.id("ticket-message", text(message, "id")), "userId": m.id("user", text(message, "user_id")), "author": email, "admin": admin, "content": text(message, "message"), "createdAt": m.created(message, "created_at"), "attachmentIds": []string{}})
		}
		state := "open"
		if integer(ticket, "status") == 1 {
			state = "closed"
		} else if len(thread) > 0 && p.Bool(thread[len(thread)-1], "admin") {
			state = "answered"
		}
		m.doc("support", "tickets", m.id("ticket", text(ticket, "id")), p.Record{"userId": user, "email": text(users[text(ticket, "user_id")], "email"), "subject": text(ticket, "subject"), "messages": thread, "status": state, "createdAt": m.created(ticket, "created_at"), "updatedAt": m.created(ticket, "updated_at", "created_at"), "source": m.ref(ticket, table), "sourceContent": ticket})
	}
	for _, item := range []struct{ table, collection, content string }{{"v2_notice", "announcements", "content"}, {"v2_knowledge", "articles", "body"}} {
		table, rows := m.table(item.table)
		for _, row := range rows {
			content := text(row, item.content)
			if image := strings.TrimSpace(text(row, "img_url")); image != "" {
				content = fmt.Sprintf("![](<%s>)\n\n%s", image, content)
			}
			m.doc("support", item.collection, m.id(item.collection, text(row, "id")), p.Record{"title": text(row, "title"), "content": content, "published": truth(row, "show"), "createdAt": m.created(row, "created_at"), "updatedAt": m.created(row, "updated_at", "created_at"), "source": m.ref(row, table), "sourceContent": row})
		}
	}
}
