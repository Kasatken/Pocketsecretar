module pocketsecret

go 1.22.4

require (
	github.com/golang-jwt/jwt/v5 v5.2.3
	github.com/lib/pq v1.10.9
	golang.org/x/crypto v0.31.0 // совместимо с Go 1.22 (сервер без Go 1.24+)
	golang.org/x/oauth2 v0.21.0
)
