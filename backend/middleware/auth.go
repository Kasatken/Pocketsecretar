package middleware

import (
	"context"
	"net/http"
	"strings"

	"pocketsecret/security"
)

type contextKey string

const UserIDContextKey contextKey = "authUserID"

func RequireAuth(tokens *security.TokenManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		claims, err := tokens.ParseAccessToken(strings.TrimPrefix(header, prefix))
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDContextKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	value := ctx.Value(UserIDContextKey)
	userID, ok := value.(int64)
	return userID, ok
}
