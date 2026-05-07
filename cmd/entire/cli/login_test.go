package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/auth"
)

// mockClient implements deviceAuthClient for unit tests.
type mockClient struct {
	responses []pollResponse
	start     *auth.DeviceAuthStart
	startErr  error
	calls     int
}

type pollResponse struct {
	result *auth.DeviceAuthPoll
	err    error
}

func (m *mockClient) StartDeviceAuth(_ context.Context) (*auth.DeviceAuthStart, error) {
	if m.startErr != nil {
		return nil, m.startErr
	}
	if m.start == nil {
		return nil, errors.New("not implemented in mock")
	}
	return m.start, nil
}

func (m *mockClient) BaseURL() string {
	return "http://test"
}

func (m *mockClient) PollDeviceAuth(_ context.Context, _ string) (*auth.DeviceAuthPoll, error) {
	if m.calls >= len(m.responses) {
		return nil, errors.New("unexpected poll call")
	}
	r := m.responses[m.calls]
	m.calls++
	return r.result, r.err
}

type mockLoginTokenStore struct {
	saveErr    error
	baseURL    string
	savedToken string
}

func (m *mockLoginTokenStore) SaveToken(baseURL, token string) error {
	m.baseURL = baseURL
	m.savedToken = token
	return m.saveErr
}

func newSuccessfulLoginClient(token string) *mockClient {
	return &mockClient{
		start: &auth.DeviceAuthStart{
			DeviceCode:      "device-1",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://entire.io/cli/auth",
			ExpiresIn:       60,
			Interval:        1,
		},
		responses: []pollResponse{
			{result: &auth.DeviceAuthPoll{AccessToken: token}},
		},
	}
}

func TestRequireNonKeyringStorageConfigured(t *testing.T) {
	t.Run("flag unset is a no-op", func(t *testing.T) {
		t.Setenv(auth.SecretsPathEnvVar, "")
		if err := requireNonKeyringStorageConfigured(false); err != nil {
			t.Fatalf("err = %v, want nil when --no-keyring is unset", err)
		}
	})

	t.Run("flag set without ENTIRE_SECRETS_PATH errors before device auth", func(t *testing.T) {
		t.Setenv(auth.SecretsPathEnvVar, "")
		err := requireNonKeyringStorageConfigured(true)
		if err == nil {
			t.Fatal("expected error when --no-keyring is set with no SECRETS_PATH")
		}
		if !strings.Contains(err.Error(), auth.SecretsPathEnvVar) {
			t.Fatalf("err = %v, want mention of %s", err, auth.SecretsPathEnvVar)
		}
	})

	t.Run("flag set with ENTIRE_SECRETS_PATH passes preflight", func(t *testing.T) {
		t.Setenv(auth.SecretsPathEnvVar, "/tmp/credentials.json")
		if err := requireNonKeyringStorageConfigured(true); err != nil {
			t.Fatalf("err = %v, want nil when SECRETS_PATH is set", err)
		}
	})

	t.Run("flag set with whitespace-only ENTIRE_SECRETS_PATH errors", func(t *testing.T) {
		t.Setenv(auth.SecretsPathEnvVar, "   ")
		if err := requireNonKeyringStorageConfigured(true); err == nil {
			t.Fatal("whitespace SECRETS_PATH must not satisfy --no-keyring")
		}
	})
}

func TestRunLogin_SaveFailureIncludesHeadlessHint(t *testing.T) {
	client := newSuccessfulLoginClient("login-token")
	store := &mockLoginTokenStore{
		saveErr: errors.New("save token to keyring: failed to unlock correct collection"),
	}

	var out, errOut bytes.Buffer
	err := runLoginWithStore(context.Background(), &out, &errOut, client,
		func(context.Context, string) error { return nil },
		store)
	if err == nil {
		t.Fatal("expected save failure")
	}
	if !strings.Contains(err.Error(), "save auth token") {
		t.Fatalf("error = %v, want save auth token context", err)
	}
	if !strings.Contains(errOut.String(), auth.SecretsPathEnvVar) {
		t.Fatalf("stderr = %q, want %s hint", errOut.String(), auth.SecretsPathEnvVar)
	}
}

func TestRunLogin_WarnsWhenEnvTokenShadowsSavedToken(t *testing.T) {
	t.Setenv(auth.AuthTokenEnvVar, "env-token")

	client := newSuccessfulLoginClient("login-token")
	store := &mockLoginTokenStore{}

	var out, errOut bytes.Buffer
	err := runLoginWithStore(context.Background(), &out, &errOut, client,
		func(context.Context, string) error { return nil },
		store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.savedToken != "login-token" {
		t.Fatalf("saved token = %q, want login-token", store.savedToken)
	}
	if !strings.Contains(errOut.String(), auth.AuthTokenEnvVar) {
		t.Fatalf("stderr = %q, want %s warning", errOut.String(), auth.AuthTokenEnvVar)
	}
	if !strings.Contains(out.String(), "Login complete.") {
		t.Fatalf("stdout = %q, want login complete", out.String())
	}
}

func TestWaitForApproval_ImmediateSuccess(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{AccessToken: "tok-123"}},
	}}

	token, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-123" {
		t.Fatalf("token = %q, want %q", token, "tok-123")
	}
	if poller.calls != 1 {
		t.Fatalf("calls = %d, want 1", poller.calls)
	}
}

func TestWaitForApproval_PendingThenSuccess(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{Error: "authorization_pending"}},
		{result: &auth.DeviceAuthPoll{Error: "authorization_pending"}},
		{result: &auth.DeviceAuthPoll{AccessToken: "tok-456"}},
	}}

	token, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-456" {
		t.Fatalf("token = %q, want %q", token, "tok-456")
	}
	if poller.calls != 3 {
		t.Fatalf("calls = %d, want 3", poller.calls)
	}
}

func TestWaitForApproval_AccessDenied(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{Error: "access_denied"}},
	}}

	_, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "device authorization denied") {
		t.Fatalf("err = %v, want 'device authorization denied'", err)
	}
}

func TestWaitForApproval_ExpiredToken(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{Error: "expired_token"}},
	}}

	_, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "device authorization expired") {
		t.Fatalf("err = %v, want 'device authorization expired'", err)
	}
}

func TestWaitForApproval_UnknownError(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{Error: "server_error"}},
	}}

	_, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "server_error") {
		t.Fatalf("err = %v, want to contain 'server_error'", err)
	}
}

func TestWaitForApproval_EmptyTokenOnSuccess(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{AccessToken: ""}},
	}}

	_, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "completed without a token") {
		t.Fatalf("err = %v, want 'completed without a token'", err)
	}
}

func TestWaitForApproval_SlowDown(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{Error: "slow_down"}},
		{result: &auth.DeviceAuthPoll{AccessToken: "tok-slow"}},
	}}

	token, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-slow" {
		t.Fatalf("token = %q, want %q", token, "tok-slow")
	}
}

func TestWaitForApproval_ExpiresInClamped(t *testing.T) {
	t.Parallel()

	// expiresIn=0 should use maxExpiresIn, not panic or return immediately.
	// We verify by checking the function still polls (doesn't error on first call).
	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{AccessToken: "tok-clamp"}},
	}}

	token, err := waitForApproval(context.Background(), poller, "device-1", 0, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-clamp" {
		t.Fatalf("token = %q, want %q", token, "tok-clamp")
	}
}

func TestWaitForApproval_NegativeExpiresInClamped(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{AccessToken: "tok-neg"}},
	}}

	token, err := waitForApproval(context.Background(), poller, "device-1", -1, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-neg" {
		t.Fatalf("token = %q, want %q", token, "tok-neg")
	}
}

func TestWaitForApproval_TransientErrorRetry(t *testing.T) {
	t.Parallel()

	poller := &mockClient{responses: []pollResponse{
		{err: errors.New("connection refused")},
		{err: errors.New("timeout")},
		{result: &auth.DeviceAuthPoll{AccessToken: "tok-retry"}},
	}}

	token, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-retry" {
		t.Fatalf("token = %q, want %q", token, "tok-retry")
	}
	if poller.calls != 3 {
		t.Fatalf("calls = %d, want 3", poller.calls)
	}
}

func TestWaitForApproval_TransientErrorExhausted(t *testing.T) {
	t.Parallel()

	var responses []pollResponse
	for range maxTransientErrors + 1 {
		responses = append(responses, pollResponse{err: errors.New("server error")})
	}
	poller := &mockClient{responses: responses}

	_, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "consecutive failures") {
		t.Fatalf("err = %v, want 'consecutive failures'", err)
	}
	if poller.calls != maxTransientErrors {
		t.Fatalf("calls = %d, want %d", poller.calls, maxTransientErrors)
	}
}

func TestWaitForApproval_TransientErrorCounterResets(t *testing.T) {
	t.Parallel()

	// 4 transient errors, then a pending response (resets counter), then 4 more, then success.
	var responses []pollResponse
	for range maxTransientErrors - 1 {
		responses = append(responses, pollResponse{err: errors.New("blip")})
	}
	responses = append(responses, pollResponse{result: &auth.DeviceAuthPoll{Error: "authorization_pending"}})
	for range maxTransientErrors - 1 {
		responses = append(responses, pollResponse{err: errors.New("blip")})
	}
	responses = append(responses, pollResponse{result: &auth.DeviceAuthPoll{AccessToken: "tok-reset"}})
	poller := &mockClient{responses: responses}

	token, err := waitForApproval(context.Background(), poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "tok-reset" {
		t.Fatalf("token = %q, want %q", token, "tok-reset")
	}
}

func TestWaitForApproval_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	poller := &mockClient{responses: []pollResponse{
		{result: &auth.DeviceAuthPoll{Error: "authorization_pending"}},
	}}

	_, err := waitForApproval(ctx, poller, "device-1", 60, time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("err = %v, want context canceled", err)
	}
}
