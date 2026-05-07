package auth

import (
	"os"
	"strings"
)

const (
	// AuthTokenEnvVar provides a read-only bearer token for non-interactive CLI auth.
	AuthTokenEnvVar = "ENTIRE_AUTH_TOKEN" // #nosec G101 -- this is an environment variable name, not a credential.

	// SecretsPathEnvVar points to an opt-in file-backed token store.
	SecretsPathEnvVar = "ENTIRE_SECRETS_PATH"
)

// TokenSource identifies where the active auth token came from.
type TokenSource string

const (
	TokenSourceNone    TokenSource = ""
	TokenSourceEnv     TokenSource = "env"
	TokenSourceFile    TokenSource = "file"
	TokenSourceKeyring TokenSource = "keyring"
)

// TokenInfo is a source-aware auth token lookup result.
type TokenInfo struct {
	Value  string
	Source TokenSource
	Path   string
}

func envAuthToken() string {
	return strings.TrimSpace(os.Getenv(AuthTokenEnvVar))
}
