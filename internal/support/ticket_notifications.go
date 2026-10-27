package support

import (
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) event(event p.Event) error {
	if event.Type != "system.tick" || event.Source != "core" {
		return fmt.Errorf("工单事件来源无效")
	}
	tickets, err := a.C.List("tickets")
	if err != nil {
		return err
	}
	for _, ticket := range tickets {
		if err = a.flushTicketNotifications(ticket); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) flushTicketNotifications(ticket p.Record) error {
	pending, _ := ticket["pendingNotifications"].([]any)
	for len(pending) > 0 {
		data, _ := pending[0].(map[string]any)
		if err := a.C.Emit("ticket.replied", p.Record(data)); err != nil {
			return err
		}
		pending = pending[1:]
		ticket["pendingNotifications"] = pending
		if err := a.C.Put("tickets", ticket); err != nil {
			return err
		}
	}
	return nil
}
