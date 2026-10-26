package commerce

import (
	"errors"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"time"
)

func (a *app) walletDebited(order p.Record) (bool, error) {
	rows, e := a.C.List("ledger")
	for _, row := range rows {
		if p.Text(row, "id") == "order-"+p.Text(order, "id") {
			return true, e
		}
	}
	return false, e
}

func (a *app) tick() error {
	commissions, e := a.C.List("commissions")
	if e != nil {
		return e
	}
	var failures []error
	for _, row := range commissions {
		if p.Text(row, "status") == "pending" && p.Text(row, "availableAt") <= c.Now() {
			row["status"] = "available"
			if e = a.C.Put("commissions", row); e != nil {
				failures = append(failures, e)
			}
		}
	}
	orders, e := a.C.List("orders")
	if e != nil {
		return e
	}
	ledger, e := a.C.List("ledger")
	if e != nil {
		return e
	}
	debited := map[string]bool{}
	for _, row := range ledger {
		debited[p.Text(row, "id")] = true
	}
	gifts, e := a.C.List("gifts")
	if e != nil {
		return e
	}
	for _, gift := range gifts {
		if p.Text(gift, "status") == "used" && p.Text(gift, "userId") != "" && !debited["gift-"+p.Text(gift, "id")] {
			if e = a.creditGift(gift); e != nil {
				failures = append(failures, e)
			}
		}
	}
	for _, row := range orders {
		var err error
		switch p.Text(row, "status") {
		case "paid":
			err = a.fulfill(row)
		case "refunding":
			err = a.refund(row)
		case "pending":
			if debited["order-"+p.Text(row, "id")] {
				row["method"] = "wallet"
				err = a.paid(row)
			} else if created, parseErr := time.Parse(time.RFC3339, p.Text(row, "createdAt")); parseErr == nil && time.Since(created) > 24*time.Hour {
				row["status"] = "canceled"
				err = a.C.Put("orders", row)
			}
		case "fulfilled":
			if p.Bool(row, "notificationPending") {
				err = a.notifyFulfilled(row)
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
	orders, e := a.C.List("orders")
	if e != nil {
		return e
	}
	var row p.Record
	for _, order := range orders {
		if p.Text(order, "id") == p.Text(data, "orderId") {
			row = order
			break
		}
	}
	if row == nil {
		return a.paymentReview(data, "找不到对应订单")
	}
	if p.Number(row, "amount") != p.Number(data, "amount") || row["currency"] != data["currency"] {
		return a.paymentReview(data, "支付金额或币种与订单不符")
	}
	samePayment := p.Text(row, "method") == "online" && p.Text(data, "id") != "" && p.Text(row, "paymentId") == p.Text(data, "id")
	if samePayment && (p.Text(row, "status") == "fulfilled" || p.Text(row, "status") == "refunded") {
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
