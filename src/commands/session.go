package commands

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"futrou-cli/src/api"
	"futrou-cli/src/logger"
	"futrou-cli/src/services"

	"github.com/urfave/cli/v2"
)

// authContext is what GET /v2/auth/context says about the token it is called with.
type authContext struct {
	APIToken struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		WorkspaceID string `json:"workspaceId"`
	} `json:"apiToken"`
	User struct {
		ID       string `json:"id"`
		Fullname string `json:"fullname"`
		Email    string `json:"email"`
		// Workspaces are the workspaces of the user. The context is open to every
		// token, so this names a workspace even when reading it is refused.
		Workspaces []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"workspaces"`
	} `json:"user"`
}

// workspaceName is the name of the user's workspace with this ID, from the
// context, or "" when the user has no such workspace.
func (c authContext) workspaceName(id string) string {
	for _, w := range c.User.Workspaces {
		if w.ID == id {
			return w.Name
		}
	}
	return ""
}

// sessionState is what the API says about a token.
type sessionState int

const (
	// sessionValid: the API knows the user behind the token.
	sessionValid sessionState = iota
	// sessionRejected: the API does not accept the token: it answers 401, or
	// answers 200 with no user, which is what it does for a token it does not
	// know (revoked, expired or from another server).
	sessionRejected
	// sessionUnverified: the API could not be asked (offline, server error).
	sessionUnverified
)

// loaderEnabled says whether a command may show a spinner: only on a terminal a
// person looks at, never in JSON mode or in a pipe, where the animation or its
// text would end up in the output. Tests replace it.
var loaderEnabled = func(c *cli.Context) bool { return isInteractiveTerminal() && !isJSON(c) }

// startLoader shows message with a spinner (for example "Checking...") while a
// command waits for the API, and returns the function that removes it. Calling
// that function more than once is fine.
func startLoader(c *cli.Context, message string) (stop func()) {
	if !loaderEnabled(c) {
		return func() {}
	}
	logger.StartLoader(message)
	return logger.StopLoader
}

// checkSession asks the API who token is. A token that is merely stored proves
// nothing: only this tells a login that works from one that does not.
func checkSession(apiUrl, token string) (sessionState, *authContext, error) {
	client := services.NewApiClientWithToken(apiUrl, token)
	var result authContext
	status, err := client.RequestInto("GET", "/v2/auth/context", nil, &result)
	switch {
	case status == http.StatusUnauthorized:
		return sessionRejected, nil, nil
	case err != nil:
		return sessionUnverified, nil, err
	case status >= 400:
		return sessionUnverified, nil, fmt.Errorf("request failed with status %d", status)
	case result.User.ID == "" && result.User.Email == "":
		return sessionRejected, nil, nil
	}
	return sessionValid, &result, nil
}

// errSessionInvalid is what a command says when the API does not know the user.
var errSessionInvalid = errors.New("you're not logged in — the stored login is no longer valid, run 'futrou login'")

// explainAuthError adds the way out to an API error that means "no logged in
// user": the API answers a token it does not know with 400 "No userId provided"
// (or 401), which tells a person nothing about what to do.
func explainAuthError(err error) error {
	var apiErr *api.APIError
	if err == nil || !errors.As(err, &apiErr) {
		return err
	}
	text := strings.ToLower(apiErr.Error())
	// a 403 or an "Unauthorized: insufficient role" is a missing permission,
	// which logging in again cannot fix
	if strings.Contains(text, "no userid provided") || strings.TrimSpace(text) == "unauthorized" {
		return fmt.Errorf("%w (%s)", errSessionInvalid, err.Error())
	}
	return err
}

// interruptGrace is how long a cancelled command has to finish (cancel its
// requests, mark the running version failed) before it is ended anyway.
var interruptGrace = 10 * time.Second

// exitNow ends the process at once; tests replace it.
var exitNow = os.Exit

// cancelOnInterrupt makes Ctrl+C (or SIGTERM) cancel every request of client, so
// a running upload stops and its version is marked failed. The command then has
// interruptGrace to finish; after that, or on a second Ctrl+C (the default
// behavior is restored by the first), the process ends at once. The returned
// function removes the handler.
func cancelOnInterrupt(client *services.ApiClient) (stop func()) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	client.SetContext(ctx)
	finished := make(chan struct{})
	go func() {
		select {
		case <-finished:
			return
		case <-ctx.Done():
		}
		cancel() // restores the default behavior: a second Ctrl+C kills the process
		logger.Warn("cancelling (up to %s; press Ctrl+C again to exit at once)", interruptGrace)
		select {
		case <-finished:
		case <-time.After(interruptGrace):
			logger.Warn("did not finish in %s: exiting", interruptGrace)
			exitNow(130)
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(finished); cancel() }) }
}
