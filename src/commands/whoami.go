package commands

import (
	"fmt"

	"github.com/urfave/cli/v2"
)

var whoamiCommand = &cli.Command{
	Name:  "whoami",
	Usage: "Display the currently authenticated user or api token info",
	Action: func(c *cli.Context) error {
		client, err := requireAuth(c)
		if err != nil {
			return err
		}
		var result struct {
			User struct {
				ID       string `json:"id"`
				Fullname string `json:"fullname"`
				Email    string `json:"email"`
			} `json:"user"`
		}
		status, err := client.RequestInto("GET", "/v2/auth/context", nil, &result)
		if err != nil {
			return err
		}
		if status >= 400 {
			return fmt.Errorf("request failed with status %d", status)
		}
		if isJSON(c) {
			return printJSON(result.User)
		}
		if result.User.Fullname != "" {
			fmt.Printf("%s <%s>\n", result.User.Fullname, result.User.Email)
		} else {
			fmt.Println(result.User.Email)
		}
		return nil
	},
}
