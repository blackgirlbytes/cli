package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const fileStoreVersion = 1

type fileStoreData struct {
	Version int               `json:"version"`
	Tokens  map[string]string `json:"tokens"`
}

func configuredSecretsPath() (string, bool, error) {
	raw := strings.TrimSpace(os.Getenv(SecretsPathEnvVar))
	if raw == "" {
		return "", false, nil
	}

	path, err := expandHome(raw)
	if err != nil {
		return "", false, err
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", false, fmt.Errorf("%s must be an absolute path after ~/ expansion", SecretsPathEnvVar)
	}

	return path, true, nil
}

func expandHome(path string) (string, error) {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func readFileStore(path string) (fileStoreData, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return newFileStoreData(), nil
	}
	if err != nil {
		return fileStoreData{}, fmt.Errorf("stat token file: %w", err)
	}
	if info.IsDir() {
		return fileStoreData{}, fmt.Errorf("token file path is a directory: %s", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fileStoreData{}, fmt.Errorf("token file %s has unsafe permissions %o; run chmod 600 %s", path, info.Mode().Perm(), path)
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is explicit user-provided credential path.
	if err != nil {
		return fileStoreData{}, fmt.Errorf("read token file: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return newFileStoreData(), nil
	}

	var store fileStoreData
	if err := json.Unmarshal(data, &store); err != nil {
		return fileStoreData{}, fmt.Errorf("parse token file: %w", err)
	}
	if store.Version == 0 {
		store.Version = fileStoreVersion
	}
	if store.Version != fileStoreVersion {
		return fileStoreData{}, fmt.Errorf("unsupported token file version %d", store.Version)
	}
	if store.Tokens == nil {
		store.Tokens = make(map[string]string)
	}

	return store, nil
}

func writeFileStore(path string, store fileStoreData) error {
	if store.Version == 0 {
		store.Version = fileStoreVersion
	}
	if store.Tokens == nil {
		store.Tokens = make(map[string]string)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create token file directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary token file: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temporary token file permissions: %w", err)
	}

	enc := json.NewEncoder(temp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(store); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write token file JSON: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary token file: %w", err)
	}

	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace token file: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("set token file permissions: %w", err)
		}
	}

	return nil
}

func newFileStoreData() fileStoreData {
	return fileStoreData{
		Version: fileStoreVersion,
		Tokens:  make(map[string]string),
	}
}

func getFileToken(path, baseURL string) (string, error) {
	store, err := readFileStore(path)
	if err != nil {
		return "", err
	}
	return store.Tokens[baseURL], nil
}

func saveFileToken(path, baseURL, token string) error {
	store, err := readFileStore(path)
	if err != nil {
		return err
	}
	store.Tokens[baseURL] = token
	if err := writeFileStore(path, store); err != nil {
		return err
	}
	return nil
}

func deleteFileToken(path, baseURL string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat token file: %w", err)
	}

	store, err := readFileStore(path)
	if err != nil {
		return err
	}
	if _, ok := store.Tokens[baseURL]; !ok {
		return nil
	}
	delete(store.Tokens, baseURL)
	if err := writeFileStore(path, store); err != nil {
		return err
	}
	return nil
}
