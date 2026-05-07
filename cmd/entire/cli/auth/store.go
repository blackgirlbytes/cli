package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/api"
	"github.com/zalando/go-keyring"
)

const keyringService = "entire-cli"

// Store manages CLI authentication tokens across the configured auth sources.
type Store struct {
	service string
}

// NewStore returns a Store using the default keyring service when no file store is configured.
func NewStore() *Store {
	return &Store{service: keyringService}
}

// NewStoreWithService returns a Store with a custom keyring service name (for testing).
func NewStoreWithService(service string) *Store {
	return &Store{service: service}
}

// SaveToken persists an access token for the given base URL.
func (s *Store) SaveToken(baseURL, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("refusing to save empty token")
	}

	if path, ok, err := configuredSecretsPath(); err != nil {
		return err
	} else if ok {
		if err := saveFileToken(path, baseURL, token); err != nil {
			return fmt.Errorf("save token to file: %w", err)
		}
		return nil
	}

	if err := keyring.Set(s.service, baseURL, token); err != nil {
		return fmt.Errorf("save token to keyring: %w", err)
	}

	return nil
}

// GetToken retrieves a stored token for the given base URL.
// Returns an empty string (and no error) if no token is stored.
func (s *Store) GetToken(baseURL string) (string, error) {
	info, err := s.GetTokenInfo(baseURL)
	if err != nil {
		return "", err
	}

	return info.Value, nil
}

// GetTokenInfo retrieves a stored token and reports where it came from.
// Returns an empty TokenInfo (and no error) if no token is stored.
func (s *Store) GetTokenInfo(baseURL string) (TokenInfo, error) {
	// ENTIRE_AUTH_TOKEN is scoped to the production API origin so a stray
	// ENTIRE_API_BASE_URL override can't leak a prod bearer to a staging
	// or local endpoint. Custom origins must use the per-origin file or
	// keyring stores, which are already keyed by baseURL.
	if baseURL == api.DefaultBaseURL {
		if token := envAuthToken(); token != "" {
			return TokenInfo{Value: token, Source: TokenSourceEnv}, nil
		}
	}

	if path, ok, err := configuredSecretsPath(); err != nil {
		return TokenInfo{}, err
	} else if ok {
		token, err := getFileToken(path, baseURL)
		if err != nil {
			return TokenInfo{}, fmt.Errorf("get token from file: %w", err)
		}
		if token == "" {
			return TokenInfo{}, nil
		}
		return TokenInfo{Value: token, Source: TokenSourceFile, Path: path}, nil
	}

	token, err := keyring.Get(s.service, baseURL)
	if errors.Is(err, keyring.ErrNotFound) {
		return TokenInfo{}, nil
	}
	if err != nil {
		return TokenInfo{}, fmt.Errorf("get token from keyring: %w", err)
	}

	return TokenInfo{Value: token, Source: TokenSourceKeyring}, nil
}

// DeleteToken removes a stored token for the given base URL.
func (s *Store) DeleteToken(baseURL string) error {
	if path, ok, err := configuredSecretsPath(); err != nil {
		return err
	} else if ok {
		if err := deleteFileToken(path, baseURL); err != nil {
			return fmt.Errorf("delete token from file: %w", err)
		}
		return nil
	}

	err := keyring.Delete(s.service, baseURL)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete token from keyring: %w", err)
	}

	return nil
}

// LookupCurrentToken retrieves the token for the current base URL.
func LookupCurrentToken() (string, error) {
	store := NewStore()
	return store.GetToken(api.BaseURL())
}
