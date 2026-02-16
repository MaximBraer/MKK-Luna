package middleware

import (
	"context"
	"net/http"
	"strings"

	"MKK-Luna/internal/service"
	"MKK-Luna/pkg/api/response"
)

type ctxKey string

const ctxUserID ctxKey = "user_id"

func UserIDFromContext(ctx context.Context) (int64, bool) {
	v := ctx.Value(ctxUserID)
	if v == nil {
		return 0, false
	}
	id, ok := v.(int64)
	return id, ok
}

func AuthMiddleware(auth *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			userID, err := auth.ParseAccessTokenCtx(r.Context(), parts[1])
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
