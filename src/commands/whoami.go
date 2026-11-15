package commands

import (
	"fmt"
	"net/url"

	"futrou-cli/src/cliconfig"
	"futrou-cli/src/constants"
	"futrou-cli/src/services"
	"futrou-cli/src/utils"

	"github.com/urfave/cli/v2"
)

// errNotLoggedIn is what whoami says when there is no token to check.
var errNotLoggedIn = fmt.Errorf("you're not logged in — run 'futrou login'")

var whoamiCommand = &cli.Command{
	Name:  "whoami",
	Usage: "Display the logged in user (or API key) and workspace (checks that the API token is still valid)",
	Action: func(c *cli.Context) error {
		apiUrl := globalApiUrl(c)
		cfg, err := cliconfig.Load()
		if err != nil {
			return err
		}
		token := globalApiToken(c)
		viaAPIKey := token != "" // given with --api-token or FUTROU_API_TOKEN, not the stored login
		if token == "" {
			token = cfg.TokenFor(apiUrl)
		}
		if token == "" {
			return errNotLoggedIn // never starts a login: whoami only reports
		}

		stopLoader := startLoader(c, "Checking...")
		defer stopLoader()
		state, context, err := checkSession(apiUrl, token)
		if err != nil {
			return err
		}
		if state == sessionRejected {
			return fmt.Errorf("you're not logged in — the API token for %s is no longer valid, run 'futrou login'", apiUrl)
		}

		info := newSessionInfo(apiUrl, token, cfg, *context, viaAPIKey)
		stopLoader() // before anything is printed
		if isJSON(c) {
			return printJSON(info.json())
		}
		fmt.Print(info.text())
		return nil
	},
}

// sessionInfo is who is logged in where: what `futrou whoami` shows, and what
// `futrou login` shows when you are already logged in.
type sessionInfo struct {
	apiUrl        string
	context       authContext
	viaAPIKey     bool
	workspaceID   string
	workspaceName string
}

// newSessionInfo adds the workspace to what the API said about the token: the
// default workspace of the login, else the one of the token, with its name when
// the API lets the token read it.
func newSessionInfo(apiUrl, token string, cfg *cliconfig.CliConfig, context authContext, viaAPIKey bool) sessionInfo {
	info := sessionInfo{apiUrl: apiUrl, context: context, viaAPIKey: viaAPIKey}
	info.workspaceID = cfg.DefaultWorkspaceFor(apiUrl)
	if info.workspaceID == "" {
		info.workspaceID = context.APIToken.WorkspaceID
	}
	if info.workspaceID != "" {
		info.workspaceName = context.workspaceName(info.workspaceID)
		if info.workspaceName == "" {
			info.workspaceName = lookupWorkspaceName(apiUrl, token, info.workspaceID)
		}
	}
	return info
}

// lookupWorkspaceName is the name of a workspace, read directly or else from the
// list of the account's workspaces, or "" when the API lets the token read neither.
func lookupWorkspaceName(apiUrl, token, id string) string {
	client := services.NewApiClientWithToken(apiUrl, token)
	var workspace struct {
		Name string `json:"name"`
	}
	if status, err := client.RequestInto("GET", "/v2/workspaces/"+url.PathEscape(id), nil, &workspace); err == nil && status < 400 && workspace.Name != "" {
		return workspace.Name
	}
	var workspaces []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if status, err := client.RequestInto("GET", "/v2/workspaces", nil, &workspaces); err == nil && status < 400 {
		for _, w := range workspaces {
			if w.ID == id {
				return w.Name
			}
		}
	}
	return ""
}

func (i sessionInfo) json() map[string]interface{} {
	output := map[string]interface{}{
		"user":      i.context.User,
		"workspace": map[string]string{"id": i.workspaceID, "name": i.workspaceName},
		"apiUrl":    i.apiUrl,
	}
	if i.viaAPIKey {
		output["apiKey"] = map[string]string{"id": i.context.APIToken.ID, "name": i.context.APIToken.Name, "type": i.context.APIToken.Type}
	}
	return output
}

func (i sessionInfo) text() string {
	return whoamiText(i.apiUrl, i.context, i.viaAPIKey, i.workspaceName, i.workspaceID)
}

// whoamiText is what whoami prints: who is logged in, then the workspace.
//
//	User: Ada Lovelace <ada@example.com>
//	Workspace: acme (019e...)
//
// A line "API URL: ..." comes first, but only when the API is not the default one.
// The user line starts with "API Key" instead of "User" for a session that uses an
// API key, followed by the name of the key.
func whoamiText(apiUrl string, result authContext, viaAPIKey bool, workspaceName, workspaceID string) string {
	label, name := "User", result.User.Fullname
	if viaAPIKey {
		label, name = "API Key", result.APIToken.Name
	}
	text := ""
	if utils.NormalizeApiUrl(apiUrl) != utils.NormalizeApiUrl(constants.ApiUrl) {
		text += "API URL: " + apiUrl + "\n" // only when it is not the default one
	}
	text += label + ": " + nameAndEmail(name, result.User.Email) + "\n"
	switch {
	case workspaceName != "":
		text += fmt.Sprintf("Workspace: %s (%s)\n", workspaceName, workspaceID)
	case workspaceID != "":
		text += "Workspace: " + workspaceID + "\n"
	default:
		text += "Workspace: none selected — pass --workspace or run 'futrou login'\n"
	}
	return text
}

// nameAndEmail is "name <email>", or only the email when there is no name (or
// the name is the email), or only the name when there is no email.
func nameAndEmail(name, email string) string {
	switch {
	case name == "" || name == email:
		return email
	case email == "":
		return name
	}
	return name + " <" + email + ">"
}
