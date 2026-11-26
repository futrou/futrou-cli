package cliconfig

import (
	"errors"
	"strings"
	"testing"
)

func TestCallbackPageSuccessAndFailure(t *testing.T) {
	ok := renderCallbackPage(nil, "dark")
	if !strings.Contains(ok, "Success") || !strings.Contains(ok, `data-theme="dark"`) || strings.Contains(ok, "{{") {
		t.Errorf("bad success page")
	}
	bad := renderCallbackPage(errors.New(`<script>x</script> invalid`), "")
	if !strings.Contains(bad, "invalid") || strings.Contains(bad, "<script>x") || strings.Contains(bad, `<html lang="en" data-theme`) {
		t.Errorf("bad failure page")
	}
}
