package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"futrou-cli/src/api"
	"futrou-cli/src/config"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

var loginCommand = &cli.Command{
	Name:  "login",
	Usage: "Authenticate with Futrou Cloud",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "email",
			Aliases: []string{"e"},
			Usage:   "Email address",
			EnvVars: []string{"FUTROU_EMAIL"},
		},
		&cli.StringFlag{
			Name:    "password",
			Aliases: []string{"p"},
			Usage:   "Password (prefer interactive prompt)",
			EnvVars: []string{"FUTROU_PASSWORD"},
		},
	},
	Action: func(c *cli.Context) error {
		email := c.String("email")
		password := c.String("password")
		apiUrl := globalApiUrl(c)

		if email == "" {
			fmt.Print("Email: ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			email = strings.TrimSpace(input)
			if email == "" {
				return fmt.Errorf("email is required")
			}
		}

		if password == "" {
			fmt.Print("Password: ")
			raw, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			password = strings.TrimSpace(string(raw))
			if password == "" {
				return fmt.Errorf("password is required")
			}
		}

		client := services.NewApiClientWithToken(apiUrl, "")
		var loginResp api.LoginResponse
		status, err := client.RequestInto("POST", "/v2/auth/login", map[string]string{
			"email":    email,
			"password": password,
		}, &loginResp)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}
		if status >= 400 {
			return fmt.Errorf("login failed (status %d)", status)
		}

		apiKey := loginResp.ApiToken.Id + "-" + loginResp.ApiToken.Token
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfg.ApiUrl = apiUrl
		cfg.SetToken(apiUrl, apiKey)
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		if isJSON(c) {
			return printJSON(map[string]string{
				"email":  loginResp.User.Email,
				"status": "logged in",
			})
		}

		fmt.Printf("✓ Logged in as %s\n", loginResp.User.Email)
		return nil
	},
}
