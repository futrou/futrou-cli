package commands

import (
	"testing"

	"futrou-cli/src/constants"
)

func TestVersionPrintsTheVersion(t *testing.T) {
	old := constants.Version
	constants.Version = "9.8.7"
	defer func() { constants.Version = old }()

	out, err := captureRun([]string{"futrou", "version"})
	assertNoError(t, err)
	assertContains(t, out, "9.8.7")

	out, err = captureRun([]string{"futrou", "--log-format", "json", "version"})
	assertNoError(t, err)
	assertContains(t, out, `\"version\": \"9.8.7\"`)
}
