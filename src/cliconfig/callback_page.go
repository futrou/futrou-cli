package cliconfig

import (
	"embed"
	"html"
	"strings"
)

//go:embed callback/page.html
var callbackAssets embed.FS

const confettiSVG = `<svg class="confetti" viewBox="0 0 300 300" fill="currentColor" aria-hidden="true"><rect x="-5" y="-2.5" width="10" height="5" rx="1.5" transform="translate(233.1 146.5) rotate(24)"/><circle cx="265.6" cy="166.2" r="2.5"/><path d="M-6 0 q3 -5 6 0 t6 0" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" transform="translate(217.1 194.1) rotate(222)"/><path d="M0 -5 L5 4 L-5 4 Z" transform="translate(211.5 202.3) rotate(217)"/><rect x="-5" y="-2.5" width="10" height="5" rx="1.5" transform="translate(226.3 238.7) rotate(114)"/><circle cx="190.2" cy="266.9" r="4"/><path d="M-6 0 q3 -5 6 0 t6 0" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" transform="translate(161.7 229.5) rotate(68)"/><path d="M0 -5 L5 4 L-5 4 Z" transform="translate(144.1 234.1) rotate(292)"/><rect x="-5" y="-2.5" width="10" height="5" rx="1.5" transform="translate(109.7 273.0) rotate(297)"/><circle cx="101.7" cy="217.5" r="4"/><path d="M-6 0 q3 -5 6 0 t6 0" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" transform="translate(99.4 205.3) rotate(254)"/><path d="M0 -5 L5 4 L-5 4 Z" transform="translate(50.3 196.9) rotate(238)"/><rect x="-5" y="-2.5" width="10" height="5" rx="1.5" transform="translate(45.7 173.4) rotate(127)"/><circle cx="16.1" cy="140.5" r="4"/><path d="M-6 0 q3 -5 6 0 t6 0" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" transform="translate(41.1 128.6) rotate(229)"/><path d="M0 -5 L5 4 L-5 4 Z" transform="translate(85.6 120.3) rotate(214)"/><rect x="-5" y="-2.5" width="10" height="5" rx="1.5" transform="translate(75.5 94.0) rotate(250)"/><circle cx="75.0" cy="45.5" r="4"/><path d="M-6 0 q3 -5 6 0 t6 0" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" transform="translate(122.6 59.3) rotate(179)"/><path d="M0 -5 L5 4 L-5 4 Z" transform="translate(138.0 28.2) rotate(35)"/><rect x="-5" y="-2.5" width="10" height="5" rx="1.5" transform="translate(169.3 56.2) rotate(340)"/><circle cx="184.1" cy="20.8" r="4"/><path d="M-6 0 q3 -5 6 0 t6 0" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" transform="translate(220.3 70.3) rotate(197)"/><path d="M0 -5 L5 4 L-5 4 Z" transform="translate(224.4 95.5) rotate(236)"/><rect x="-5" y="-2.5" width="10" height="5" rx="1.5" transform="translate(251.6 92.1) rotate(30)"/><circle cx="238.6" cy="121.7" r="3.2"/></svg>`

const (
	glyphOK  = `<svg viewBox="0 0 24 24"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>`
	glyphBad = `<svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18"/></svg>`
)

// renderCallbackPage returns the single self-contained HTML page (styles and
// images embedded) shown in the browser when the login ends. A nil failure is
// the success page, otherwise the page names the reason. theme is "light" or
// "dark" (what the Futrou app was showing, sent in the redirect URL); anything
// else follows the operating system's setting.
func renderCallbackPage(failure error, theme string) string {
	read := func(name string) string {
		b, _ := callbackAssets.ReadFile("callback/" + name)
		return string(b)
	}

	themeAttr := ""
	if theme == "light" || theme == "dark" {
		themeAttr = ` data-theme="` + theme + `"`
	}

	confetti := confettiSVG
	kind, glyph, title, body := "ok", glyphOK, "Success", `<p>You were successfully signed in to the Futrou CLI.<br>You can close this window and go back to your terminal.</p>`
	if failure != nil {
		kind, glyph, title, confetti = "bad", glyphBad, "Login failed", ""
		body = `<div class="reason">` + html.EscapeString(failure.Error()) + `</div>`
	}

	return strings.NewReplacer(
		"{{THEME_ATTR}}", themeAttr,
		"{{TITLE}}", title,
		"{{KIND}}", kind,
		"{{GLYPH}}", glyph,
		"{{CONFETTI}}", confetti,
		"{{BODY}}", body,
	).Replace(read("page.html"))
}
