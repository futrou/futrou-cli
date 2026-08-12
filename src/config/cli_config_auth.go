// Package config additions: session lifecycle (login/logout/whoami) built
// on top of CliConfig, using raw net/http rather than the services package
// to avoid an import cycle (services already imports cliconfig -> config).
package config

import "errors"

// WhoamiUser is the identity returned by CliConfig.Whoami.
type WhoamiUser struct {
	ID       string
	Fullname string
	Email    string
}

// ErrCIRequiresToken is returned by EnsureLoggedIn (and Login, when no
// token is available under CI) instead of attempting an interactive
// browser flow that nobody in a CI environment could complete.
var ErrCIRequiresToken = errors.New("not logged in — CI environment requires a non-empty --api-token (or FUTROU_API_TOKEN)")
