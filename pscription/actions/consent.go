package actions

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// consentCookies are cookies that widely-deployed consent/age-gate plugins
// read on first load. Seeding them before navigation means the dialog never
// renders — the strongest dismissal technique available. Values mark the
// banner as "already answered"; nothing here grants marketing consent
// beyond what hiding the banner implies.
var consentCookies = map[string]string{
	// OneTrust: banner suppressed when the alert box was closed before.
	"OptanonAlertBoxClosed": time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02T15:04:05.000Z"),
	// WordPress "Cookie Notice" plugin.
	"cookie_notice_accepted": "true",
	// CookieYes / "GDPR Cookie Consent" (very common on WordPress).
	"viewed_cookie_policy":             "yes",
	"cookielawinfo-checkbox-necessary": "yes",
	// Complianz.
	"cmplz_banner-status": "dismissed",
	// EU cookie bar plugin family.
	"cb-enabled": "accepted",
	// WordPress "Age Gate" plugin stores the visitor's age.
	"age_gate": "21",
	// Common hand-rolled age checks.
	"age_verified":  "1",
	"ageVerified":   "true",
	"age_confirmed": "1",
}

// PreseedConsentCookies sets well-known consent/age-gate cookies for the
// target URL's host before navigation. Pre-navigation action.
func PreseedConsentCookies(target string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		u, err := url.Parse(target)
		if err != nil {
			return errors.Wrapf(err, "cannot parse target URL %q", target)
		}
		if u.Host == "" {
			// file: and other hostless URLs have no cookie jar to seed. This
			// used to be an error, and as a pre-navigation action it failed
			// the whole session: a page on disk could not be captured.
			return nil
		}
		host := u.Hostname()
		// Cover both the exact host and the registrable parent so
		// www.example.com and example.com plugins both see the cookie.
		domains := []string{host}
		if trimmed := strings.TrimPrefix(host, "www."); trimmed != host {
			domains = append(domains, trimmed)
		}

		set := 0
		for _, domain := range domains {
			for name, value := range consentCookies {
				err := network.SetCookie(name, value).
					WithDomain(domain).
					WithPath("/").
					Do(ctx)
				if err == nil {
					set++
				}
			}
		}
		log.With("cookies", set).Debug("pre-seeded consent cookies")
		return nil
	}
}

// PreseedConsentStorage registers a script that runs on every new document
// BEFORE the page's own JavaScript, seeding localStorage flags that consent
// and age-gate scripts check on startup. Pre-navigation action; survives
// reloads and same-tab navigations.
func PreseedConsentStorage() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		script, err := js.Files.ReadFile(string(js.PreseedConsent))
		if err != nil {
			return errors.Wrap(err, "failed to read preseed script")
		}
		// The script file is a parenthesized function; invoke it.
		source := fmt.Sprintf("(%s)();", script)
		// Note: errors.Wrap(nil, ...) in this errors package returns a
		// non-nil error, so only wrap actual failures.
		if _, err := page.AddScriptToEvaluateOnNewDocument(source).Do(ctx); err != nil {
			return errors.Wrap(err, "failed to register consent preseed script")
		}
		return nil
	}
}
