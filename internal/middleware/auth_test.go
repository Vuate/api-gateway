package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret"

func makeToken(secret string, userID float64, exp time.Time) string {
	claims := jwt.MapClaims{
		"user_id": userID,
		"jti":     "test-jti-123",
		"exp":     exp.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(secret))
	return signed
}

func authOKHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestJWTAuth_NoToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(authOKHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("beklenen 401, gelen %d", rec.Code)
	}
}

func TestJWTAuth_ValidToken(t *testing.T) {
	token := makeToken(testSecret, 1, time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(authOKHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("beklenen 200, gelen %d", rec.Code)
	}
}

func TestJWTAuth_InvalidSignature(t *testing.T) {
	token := makeToken("wrong-secret", 1, time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(authOKHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("beklenen 401, gelen %d", rec.Code)
	}
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	token := makeToken(testSecret, 1, time.Now().Add(-time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(authOKHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("beklenen 401, gelen %d", rec.Code)
	}
}

func TestJWTAuth_TokenFromQueryParam(t *testing.T) {
	token := makeToken(testSecret, 1, time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/?token="+token, nil)
	rec := httptest.NewRecorder()

	JWTAuth(testSecret)(http.HandlerFunc(authOKHandler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("beklenen 200, gelen %d", rec.Code)
	}
}

func TestJWTAuth_UserIDPropagated(t *testing.T) {
	token := makeToken(testSecret, 42, time.Now().Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	var gotUserID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-ID")
		w.WriteHeader(http.StatusOK)
	})

	JWTAuth(testSecret)(handler).ServeHTTP(rec, req)

	if gotUserID != "42" {
		t.Errorf("beklenen X-User-ID=42, gelen %s", gotUserID)
	}
}
