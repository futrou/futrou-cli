package config

import (
	"futrou-cli/src/constants"
	"futrou-cli/src/utils"
)

func encryptToken(plaintext string) string {
	return utils.EncryptToken(plaintext, constants.Name)
}

func decryptToken(stored string) (string, error) {
	return utils.DecryptToken(stored, constants.Name)
}
