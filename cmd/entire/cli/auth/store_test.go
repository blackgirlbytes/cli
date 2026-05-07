package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/api"
	"github.com/zalando/go-keyring"
)

const (
	testLocalToken = "local-token"
	testEnvToken   = "env-token"
	testWindowsOS  = "windows"
)

func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

func TestStoreSaveAndGetToken(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	store := NewStoreWithService("test-save-get")

	if err := store.SaveToken("https://entire.io", "prod-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	got, err := store.GetToken("https://entire.io")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "prod-token" {
		t.Fatalf("GetToken() = %q, want %q", got, "prod-token")
	}
}

func TestStoreGetToken_NotFound(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	store := NewStoreWithService("test-not-found")

	got, err := store.GetToken("https://missing.example.com")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "" {
		t.Fatalf("GetToken() = %q, want empty string", got)
	}
}

func TestStoreGetTokenInfo_EnvTokenTakesPrecedence(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	t.Setenv(AuthTokenEnvVar, " env-token ")

	store := NewStoreWithService("test-env-precedence")
	if err := store.SaveToken("https://entire.io", "keyring-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	info, err := store.GetTokenInfo("https://entire.io")
	if err != nil {
		t.Fatalf("GetTokenInfo() error = %v", err)
	}
	if info.Value != testEnvToken || info.Source != TokenSourceEnv {
		t.Fatalf("GetTokenInfo() = %#v, want env token/source", info)
	}
}

func TestStoreGetTokenInfo_WhitespaceEnvTokenIgnored(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	t.Setenv(AuthTokenEnvVar, "   ")

	store := NewStoreWithService("test-env-blank")
	if err := store.SaveToken("https://entire.io", "keyring-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	info, err := store.GetTokenInfo("https://entire.io")
	if err != nil {
		t.Fatalf("GetTokenInfo() error = %v", err)
	}
	if info.Value != "keyring-token" || info.Source != TokenSourceKeyring {
		t.Fatalf("GetTokenInfo() = %#v, want keyring token/source", info)
	}
}

func TestStoreGetTokenInfo_EnvTokenScopedToDefaultBaseURL(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	t.Setenv(AuthTokenEnvVar, testEnvToken)

	store := NewStoreWithService("test-env-scope")
	if err := store.SaveToken("http://localhost:8787", "local-keyring-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	// Custom base URL must NOT receive ENTIRE_AUTH_TOKEN — falls through to
	// the per-origin keyring/file store so a prod bearer can't leak to a
	// staging/dev override.
	info, err := store.GetTokenInfo("http://localhost:8787")
	if err != nil {
		t.Fatalf("GetTokenInfo(custom) error = %v", err)
	}
	if info.Value != "local-keyring-token" || info.Source != TokenSourceKeyring {
		t.Fatalf("GetTokenInfo(custom) = %#v, want local-keyring-token/keyring", info)
	}

	// Default base URL still gets ENTIRE_AUTH_TOKEN.
	info, err = store.GetTokenInfo(api.DefaultBaseURL)
	if err != nil {
		t.Fatalf("GetTokenInfo(default) error = %v", err)
	}
	if info.Value != testEnvToken || info.Source != TokenSourceEnv {
		t.Fatalf("GetTokenInfo(default) = %#v, want env-token/env", info)
	}
}

func TestStoreSaveToken_PreservesOtherBaseURLs(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	store := NewStoreWithService("test-preserve")

	if err := store.SaveToken("https://entire.io", "prod-token"); err != nil {
		t.Fatalf("SaveToken(prod) error = %v", err)
	}

	if err := store.SaveToken("http://localhost:8787", testLocalToken); err != nil {
		t.Fatalf("SaveToken(local) error = %v", err)
	}

	prod, err := store.GetToken("https://entire.io")
	if err != nil {
		t.Fatalf("GetToken(prod) error = %v", err)
	}
	if prod != "prod-token" {
		t.Fatalf("prod token = %q, want %q", prod, "prod-token")
	}

	local, err := store.GetToken("http://localhost:8787")
	if err != nil {
		t.Fatalf("GetToken(local) error = %v", err)
	}
	if local != testLocalToken {
		t.Fatalf("local token = %q, want %q", local, testLocalToken)
	}
}

func TestStoreSaveToken_RejectsEmptyToken(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	store := NewStoreWithService("test-empty")

	if err := store.SaveToken("https://entire.io", ""); err == nil {
		t.Fatal("SaveToken() with empty token should fail")
	}

	if err := store.SaveToken("https://entire.io", "   "); err == nil {
		t.Fatal("SaveToken() with whitespace token should fail")
	}
}

func TestStoreSaveToken_TrimsWhitespace(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	store := NewStoreWithService("test-trim")

	if err := store.SaveToken("https://entire.io", "  my-token  "); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	got, err := store.GetToken("https://entire.io")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "my-token" {
		t.Fatalf("GetToken() = %q, want %q (whitespace should be trimmed)", got, "my-token")
	}
}

func TestStoreDeleteToken(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	store := NewStoreWithService("test-delete")

	if err := store.SaveToken("https://entire.io", "tok"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	if err := store.DeleteToken("https://entire.io"); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}

	got, err := store.GetToken("https://entire.io")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "" {
		t.Fatalf("GetToken() after delete = %q, want empty", got)
	}
}

func TestStoreDeleteToken_NotFoundIsNoop(t *testing.T) {
	// Not parallel: go-keyring's mock provider uses an unprotected map.
	store := NewStoreWithService("test-delete-noop")

	if err := store.DeleteToken("https://nonexistent.example.com"); err != nil {
		t.Fatalf("DeleteToken() on missing key error = %v", err)
	}
}

func newFileBackendTestStore(t *testing.T, service string) (*Store, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "credentials.json")
	t.Setenv(SecretsPathEnvVar, path)

	return NewStoreWithService(service), path
}

func TestStoreFileBackend_SaveGetDelete(t *testing.T) {
	// Not parallel: auth env vars are process-global.
	store, path := newFileBackendTestStore(t, "test-file-backend")
	if err := store.SaveToken("https://entire.io", "file-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	info, err := store.GetTokenInfo("https://entire.io")
	if err != nil {
		t.Fatalf("GetTokenInfo() error = %v", err)
	}
	if info.Value != "file-token" || info.Source != TokenSourceFile || info.Path != path {
		t.Fatalf("GetTokenInfo() = %#v, want file token/source/path", info)
	}

	if runtime.GOOS != testWindowsOS {
		stat, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("Stat() error = %v", statErr)
		}
		if got := stat.Mode().Perm(); got != 0o600 {
			t.Fatalf("credential file mode = %o, want 600", got)
		}
	}

	if err := store.DeleteToken("https://entire.io"); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}
	got, err := store.GetToken("https://entire.io")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "" {
		t.Fatalf("GetToken() after delete = %q, want empty", got)
	}
}

func TestStoreFileBackend_PreservesOtherBaseURLs(t *testing.T) {
	// Not parallel: auth env vars are process-global.
	store, _ := newFileBackendTestStore(t, "test-file-preserve")
	if err := store.SaveToken("https://entire.io", "prod-token"); err != nil {
		t.Fatalf("SaveToken(prod) error = %v", err)
	}
	if err := store.SaveToken("http://localhost:8787", testLocalToken); err != nil {
		t.Fatalf("SaveToken(local) error = %v", err)
	}
	if err := store.DeleteToken("https://entire.io"); err != nil {
		t.Fatalf("DeleteToken(prod) error = %v", err)
	}

	got, err := store.GetToken("http://localhost:8787")
	if err != nil {
		t.Fatalf("GetToken(local) error = %v", err)
	}
	if got != testLocalToken {
		t.Fatalf("local token = %q, want %q", got, testLocalToken)
	}
}

func TestStoreFileBackend_EnvTokenTakesPrecedence(t *testing.T) {
	// Not parallel: auth env vars are process-global.
	store, _ := newFileBackendTestStore(t, "test-file-env-precedence")
	t.Setenv(AuthTokenEnvVar, testEnvToken)

	if err := store.SaveToken("https://entire.io", "file-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	info, err := store.GetTokenInfo("https://entire.io")
	if err != nil {
		t.Fatalf("GetTokenInfo() error = %v", err)
	}
	if info.Value != testEnvToken || info.Source != TokenSourceEnv {
		t.Fatalf("GetTokenInfo() = %#v, want env token/source", info)
	}
}

func TestStoreFileBackend_RejectsRelativePath(t *testing.T) {
	// Not parallel: auth env vars are process-global.
	t.Setenv(SecretsPathEnvVar, "credentials.json")

	store := NewStoreWithService("test-file-relative")
	if _, err := store.GetTokenInfo("https://entire.io"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("GetTokenInfo() err = %v, want absolute-path error", err)
	}
	if err := store.SaveToken("https://entire.io", "tok"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("SaveToken() err = %v, want absolute-path error", err)
	}
	if err := store.DeleteToken("https://entire.io"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("DeleteToken() err = %v, want absolute-path error", err)
	}
}

func TestStoreFileBackend_RejectsMalformedJSON(t *testing.T) {
	// Not parallel: auth env vars are process-global.
	store, path := newFileBackendTestStore(t, "test-file-malformed")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := store.GetTokenInfo("https://entire.io"); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("GetTokenInfo() err = %v, want parse error", err)
	}
}

func TestStoreFileBackend_RejectsGroupReadableFile(t *testing.T) {
	if runtime.GOOS == testWindowsOS {
		t.Skip("permission bit checks are Unix-specific")
	}
	// Not parallel: auth env vars are process-global.
	store, path := newFileBackendTestStore(t, "test-file-perms")
	if err := os.WriteFile(path, []byte(`{"version":1,"tokens":{"https://entire.io":"tok"}}`), 0o640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := store.GetTokenInfo("https://entire.io"); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("GetTokenInfo() err = %v, want chmod 600 hint", err)
	}
}

func TestStoreFileBackend_DeleteMissingFileDoesNotCreateFile(t *testing.T) {
	// Not parallel: auth env vars are process-global.
	store, path := newFileBackendTestStore(t, "test-file-delete-missing")
	if err := store.DeleteToken("https://entire.io"); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat() err = %v, want file to remain absent", err)
	}
}

func TestStoreFileBackend_DeleteMissingTokenPreservesExistingToken(t *testing.T) {
	// Not parallel: auth env vars are process-global.
	store, _ := newFileBackendTestStore(t, "test-file-delete-missing-token")
	if err := store.SaveToken("https://entire.io", "file-token"); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	if err := store.DeleteToken("https://missing.example.com"); err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}

	got, err := store.GetToken("https://entire.io")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if got != "file-token" {
		t.Fatalf("GetToken() = %q, want existing token preserved", got)
	}
}

func TestLookupCurrentToken(t *testing.T) {
	t.Setenv(api.BaseURLEnvVar, "http://localhost:8787")

	store := NewStore()
	if err := store.SaveToken("http://localhost:8787", testLocalToken); err != nil {
		t.Fatalf("SaveToken() error = %v", err)
	}

	got, err := LookupCurrentToken()
	if err != nil {
		t.Fatalf("LookupCurrentToken() error = %v", err)
	}
	if got != testLocalToken {
		t.Fatalf("LookupCurrentToken() = %q, want %q", got, testLocalToken)
	}
}
