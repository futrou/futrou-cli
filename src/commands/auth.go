package commands

import (
	"fmt"

	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

// requireAuth returns an error with a clear message when no API key is configured.
func requireAuth(c *cli.Context) (*services.ApiClient, error) {
	apiUrl := globalApiUrl(c)
	apiKey := globalApiKey(c)

	client, err := services.NewApiClient(apiUrl, apiKey)
	if err != nil {
		return nil, err
	}

	if client.Token() == "" {
		return nil, fmt.Errorf("not logged in — run 'futrou login' or set FUTROU_API_TOKEN")
	}

	return client, nil
}
