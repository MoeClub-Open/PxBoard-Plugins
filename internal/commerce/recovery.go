package commerce

import (
	"errors"
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"time"
)

func (a *app) walletDebited(order p.Record) (bool, error) {
	rows, e := a.QueryAll("ledger", p.Record{"field": "id", "value": "order-" + p.Text(order, "id")})
	return len(rows) > 0, e
}

func (a *app) tick() error {
	config, err := a.config()
	if err != nil {
		return err
	}
	timeout := paymentTimeout(config)
	var failures []error
	orders := map[string]p.Record{}
	filters := []p.Record{{"field": "status", "value": "paid"}, {"field": "status", "value": "refunding"}, {"field": "status", "value": "pending"}}
	for _, filter := range filters {
		rows, e := a.QueryAll("orders", filter)
		if e != nil {
			return e
		}
		for _, row := range rows {
			orders[p.Text(row, "id")] = row
		}
	}
	for _, row := range orders {
		if p.Bool(row, "imported") {
			continue
		}
		var err error
		switch p.Text(row, "status") {
		case "paid":
			err = a.fulfill(row)
		case "refunding":
			err = a.refund(row)
		case "pending":
			debited, checkErr := a.walletDebited(row)
			if checkErr != nil {
				err = checkErr
			} else if debited {
				row["method"] = "wallet"
				err = a.paid(row)
			} else if orderExpired(row, timeout, time.Now()) {
				err = a.cancelOrder(row, "timeout", time.Now())
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("订单 %s：%w", p.Text(row, "number"), err))
		}
	}
	return errors.Join(failures...)
}

func (a *app) paymentReview(data p.Record, reason string) error {
	return a.C.Emit("payment.review-required", p.Record{"paymentId": data["id"], "transactionId": data["transactionId"], "orderId": data["orderId"], "reason": reason})
}

func (a *app) paymentSucceeded(data p.Record) error {
	orders, e := a.QueryAll("orders", p.Record{"field": "id", "value": p.Text(data, "orderId")})
	if e != nil {
		return e
	}
	var row p.Record
	if len(orders) > 0 {
		row = orders[0]
	}
	if row == nil {
		return a.paymentReview(data, "找不到对应订单")
	}
	if p.Number(row, "amount") != p.Number(data, "amount") || row["currency"] != data["currency"] {
		return a.paymentReview(data, "支付金额或币种与订单不符")
	}
	config, e := a.config()
	if e != nil {
		return e
	}
	if _, e = a.expirePending(row, paymentTimeout(config), time.Now()); e != nil {
		return e
	}
	samePayment := p.Text(row, "method") == "online" && p.Text(data, "id") != "" && p.Text(row, "paymentId") == p.Text(data, "id")
	if samePayment && (p.Text(row, "status") == "fulfilled" || p.Text(row, "status") == "credited" || p.Text(row, "status") == "refunded") {
		return nil
	}
	if p.Text(row, "status") != "pending" && !(p.Text(row, "status") == "paid" && samePayment) {
		return a.paymentReview(data, "订单已关闭或已通过其他付款完成，请核对多收款")
	}
	debited, e := a.walletDebited(row)
	if e != nil {
		return e
	}
	if debited {
		if e = a.paymentReview(data, "订单已经扣除余额，请核对额外在线收款"); e != nil {
			return e
		}
		row["method"] = "wallet"
		return a.paid(row)
	}
	row["method"] = "online"
	row["paymentId"] = data["id"]
	row["transactionId"] = data["transactionId"]
	return a.paid(row)
}
