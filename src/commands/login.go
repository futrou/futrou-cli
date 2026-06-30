package commands

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"futrou-cli/src/config"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

var loginCommand = &cli.Command{
	Name:  "login",
	Usage: "Authenticate with Futrou Cloud via browser",
	Action: func(c *cli.Context) error {
		apiUrl := services.NormalizeApiUrl(globalApiUrl(c))

		discovery, err := fetchOAuthDiscovery(apiUrl)
		if err != nil {
			return fmt.Errorf("fetching OAuth config: %w", err)
		}

		clientID, err := registerClient(discovery.RegistrationEndpoint)
		if err != nil {
			return fmt.Errorf("registering OAuth client: %w", err)
		}

		verifier, challenge, err := pkce()
		if err != nil {
			return fmt.Errorf("generating PKCE: %w", err)
		}

		// Start local callback server on a random available port.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("starting local server: %w", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

		codeCh := make(chan string, 1)
		errCh := make(chan error, 1)

		srv := &http.Server{}
		http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			if errParam := r.URL.Query().Get("error"); errParam != "" {
				errCh <- fmt.Errorf("authorization denied: %s", errParam)
				fmt.Fprintf(w, "<html><body><h2>Authorization denied. You can close this window.</h2></body></html>")
				return
			}
			if code == "" {
				errCh <- fmt.Errorf("no code in callback")
				return
			}
			codeCh <- code
			fmt.Fprintf(w, "<html><body><h2>Logged in! You can close this window.</h2></body></html>")
		})

		go srv.Serve(listener)
		defer srv.Shutdown(context.Background())

		authURL := buildAuthURL(discovery.AuthorizationEndpoint, clientID, redirectURI, challenge)

		fmt.Println("Opening browser for login...")
		fmt.Printf("\nIf your browser did not open, visit:\n  %s\n\n", authURL)
		openBrowser(authURL)

		var code string
		select {
		case code = <-codeCh:
		case err = <-errCh:
			return err
		case <-time.After(5 * time.Minute):
			return fmt.Errorf("login timed out — no response received within 5 minutes")
		}

		token, userEmail, err := exchangeCode(discovery.TokenEndpoint, clientID, code, verifier, redirectURI)
		if err != nil {
			return fmt.Errorf("exchanging code for token: %w", err)
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfg.ApiUrl = apiUrl
		cfg.SetToken(apiUrl, token)
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		if isJSON(c) {
			return printJSON(map[string]string{
				"email":  userEmail,
				"status": "logged in",
			})
		}

		if userEmail != "" {
			fmt.Printf("✓ Logged in as %s\n", userEmail)
		} else {
			fmt.Println("✓ Logged in successfully")
		}
		return nil
	},
}

type oauthDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

func fetchOAuthDiscovery(apiUrl string) (*oauthDiscovery, error) {
	resp, err := http.Get(apiUrl + "/.well-known/oauth-authorization-server")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d oauthDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

func registerClient(registrationEndpoint string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"client_name":    "Futrou CLI",
		"redirect_uris":  []string{"http://localhost"},
		"grant_types":    []string{"authorization_code"},
		"response_types": []string{"code"},
	})
	resp, err := http.Post(registrationEndpoint, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var reg struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return "", err
	}
	if reg.ClientID == "" {
		return "", fmt.Errorf("no client_id in registration response")
	}
	return reg.ClientID, nil
}

func pkce() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

func buildAuthURL(authEndpoint, clientID, redirectURI, challenge string) string {
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return authEndpoint + "?" + params.Encode()
}

func exchangeCode(tokenEndpoint, clientID, code, verifier, redirectURI string) (token, email string, err error) {
	params := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	}
	resp, err := http.PostForm(tokenEndpoint, params)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	if result.AccessToken == "" {
		return "", "", fmt.Errorf("no access_token in token response")
	}
	return result.AccessToken, result.Email, nil
}

func openBrowser(u string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	cmd.Start()
}
