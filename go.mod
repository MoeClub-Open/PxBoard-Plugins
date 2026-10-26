module github.com/MoeClub-Open/PxBoard-Plugins

go 1.26.0

require (
	github.com/MoeClub-Open/PxBoard v0.4.0
	github.com/go-sql-driver/mysql v1.9.3
	golang.org/x/oauth2 v0.37.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
)

replace github.com/MoeClub-Open/PxBoard => ../PxBoard
