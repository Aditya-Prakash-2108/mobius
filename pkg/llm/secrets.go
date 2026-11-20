package llm

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const secretService = "mobius"

func secretAccount(id string) string {
	return "api-key/" + id
}

func putAPIKey(id, key string) error {
	if id == "" || key == "" {
		return nil
	}
	if err := keyring.Set(secretService, secretAccount(id), key); err != nil {
		return err
	}
	return nil
}

func getAPIKey(id string) (string, error) {
	if id == "" {
		return "", nil
	}
	key, err := keyring.Get(secretService, secretAccount(id))
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return key, nil
}
