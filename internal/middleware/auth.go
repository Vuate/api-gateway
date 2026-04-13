package middleware

import (
	"context"
	"net/http"
	"strings"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserIDKey contextKey = "userID"

func JWTAuth(secret string) func(http.Handler) http.Handler {
	if secret == "" {
		secret = "default-secret-change-in-production"
	}
	return func(next http.Handler) http.Handler {

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string

			// Header'dan oku (REST için)
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			} else if q := r.URL.Query().Get("token"); q != "" {
				// Query param'dan oku (WebSocket için — tarayıcı header gönderemiyor)
				tokenStr = q
			} else {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})

			if err != nil || !token.Valid {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

				userIDFloat, ok := claims["user_id"].(float64)
				if !ok {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
				userID := fmt.Sprintf("%d", int64(userIDFloat))
				if userID == "" {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}

			r.Header.Set("X-User-ID", userID)
			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
