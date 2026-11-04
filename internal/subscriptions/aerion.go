package subscriptions

import p "github.com/MoeClub-Open/PxBoard/pkg/plugin"

func aerion(n p.Record, cred, uuid string) p.Record {
	if p.Text(n, "protocol") == "flux" {
		return fluxClash(n, cred, uuid)
	}
	return clash(n, cred, uuid)
}
