package insightai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestVerifyConnectionClassifiesSuccessAndAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"secret must not escape"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-test","output":[]}`))
	}))
	defer server.Close()
	settings := ServiceSettings{Provider: ProviderOpenAI, BaseURL: server.URL, Model: "test-model", Timeout: 5 * time.Second, APIKey: "valid-secret"}
	success := VerifyConnection(context.Background(), settings, server.Client())
	if !success.OK || success.Status != VerificationVerified || success.HTTPStatus != http.StatusOK {
		t.Fatalf("unexpected success: %+v", success)
	}
	settings.APIKey = "invalid-secret"
	failure := VerifyConnection(context.Background(), settings, server.Client())
	if failure.OK || failure.ErrorCode != "authentication" || failure.Message == "secret must not escape" {
		t.Fatalf("unexpected authentication failure: %+v", failure)
	}
}

func TestVerifyConnectionClassifiesRateLimitAndInvalidResponse(t *testing.T) {
	rateLimited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer rateLimited.Close()
	settings := ServiceSettings{BaseURL: rateLimited.URL, APIKey: "secret", Timeout: 5 * time.Second}
	result := VerifyConnection(context.Background(), settings, rateLimited.Client())
	if result.ErrorCode != "rate_limit" {
		t.Fatalf("unexpected rate limit result: %+v", result)
	}
	invalid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer invalid.Close()
	settings.BaseURL = invalid.URL
	result = VerifyConnection(context.Background(), settings, invalid.Client())
	if result.ErrorCode != "invalid_response" {
		t.Fatalf("unexpected invalid response result: %+v", result)
	}
	missingID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"output":[]}`))
	}))
	defer missingID.Close()
	settings.BaseURL = missingID.URL
	result = VerifyConnection(context.Background(), settings, missingID.Client())
	if result.ErrorCode != "invalid_response" {
		t.Fatalf("unexpected missing response id result: %+v", result)
	}
}

func TestVerifyConnectionClassifiesTimeoutAndNetworkFailure(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"id":"late-response","output":[]}`))
	}))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	settings := ServiceSettings{BaseURL: slow.URL, APIKey: "secret", Timeout: 5 * time.Second}
	result := VerifyConnection(ctx, settings, slow.Client())
	if result.ErrorCode != "timeout" {
		t.Fatalf("unexpected timeout result: %+v", result)
	}

	networkClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed with secret=must-not-escape")
	})}
	settings.BaseURL = "https://example.invalid/v1"
	result = VerifyConnection(context.Background(), settings, networkClient)
	if result.ErrorCode != "network" || result.Message == "dial failed with secret=must-not-escape" {
		t.Fatalf("unexpected network result: %+v", result)
	}
}
