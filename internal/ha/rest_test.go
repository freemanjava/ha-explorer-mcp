package ha

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetConfig_ValidToken_ReturnsDecodedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"2026.8.0","location_name":"Home"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := GetConfig(ctx, srv.Client(), srv.URL, testToken)
	if err != nil {
		t.Fatalf("GetConfig: unexpected error: %v", err)
	}
	if got["version"] != "2026.8.0" {
		t.Fatalf("GetConfig: got %v, want version 2026.8.0", got)
	}
}

func TestGetConfig_WrongToken_ReturnsTypedAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := GetConfig(ctx, srv.Client(), srv.URL, "wrong-token")
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("GetConfig: got %v, want ErrAuthFailed", err)
	}
}

func TestGetConfig_ServerUnreachable_ReturnsUpstreamUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := GetConfig(ctx, http.DefaultClient, "http://127.0.0.1:1", testToken)
	if !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("GetConfig: got %v, want ErrUpstreamUnavailable", err)
	}
}
