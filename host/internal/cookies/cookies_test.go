package cookies

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteNetscape(t *testing.T) {
	cases := []struct {
		name   string
		cookie Cookie
		line   string
	}{
		{"domain secure HTTPOnly", Cookie{Domain: ".youtube.com", Secure: true, HTTPOnly: true, ExpirationDate: 1234567890}, "#HttpOnly_.youtube.com\tTRUE\t/\tTRUE\t1234567890\tSID\tdummy\n"},
		{"domain insecure", Cookie{Domain: ".youtube.com"}, ".youtube.com\tTRUE\t/\tFALSE\t0\tSID\tdummy\n"},
		{"host secure", Cookie{Domain: "www.youtube.com", Secure: true}, "www.youtube.com\tFALSE\t/\tTRUE\t0\tSID\tdummy\n"},
		{"host insecure", Cookie{Domain: "www.youtube.com"}, "www.youtube.com\tFALSE\t/\tFALSE\t0\tSID\tdummy\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			cookie := testCase.cookie
			cookie.Name, cookie.Value, cookie.Path = "SID", "dummy", "/"
			var output bytes.Buffer
			if err := writeNetscape(&output, []Cookie{cookie}); err != nil {
				t.Fatal(err)
			}
			if output.String() != netscapeHeader+testCase.line {
				t.Fatalf("unexpected cookie output: %s", output.String())
			}
		})
	}
}

func TestValidateRejectsUnsafeCookies(t *testing.T) {
	cases := []struct {
		cookie Cookie
		want   string
	}{
		{Cookie{Domain: ".evil.example", Name: "SID", Value: "x", Path: "/"}, "disallowed domain"},
		{Cookie{Domain: "github.com\tTRUE\t/\tFALSE\t0\tx\n.youtube.com", Name: "SID", Value: "x", Path: "/"}, "disallowed domain"},
		{Cookie{Domain: ".youtube.com", Name: "bad\tname", Value: "x", Path: "/"}, "invalid name"},
		{Cookie{Domain: ".youtube.com", Name: "SID", Value: "line\nbreak", Path: "/"}, "invalid value"},
		{Cookie{Domain: ".youtube.com", Name: "SID", Value: "x", Path: "relative"}, "invalid path"},
		{Cookie{Domain: ".youtube.com", Name: "SID", Value: "x", Path: "/", ExpirationDate: -1}, "invalid expiration"},
	}
	for _, testCase := range cases {
		err := Validate([]Cookie{testCase.cookie})
		if err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Errorf("expected %q for %#v, got %v", testCase.want, testCase.cookie, err)
		}
	}
}

func TestNormalizeDomainKeepsTheDomainsChromeReports(t *testing.T) {
	cases := []string{"youtube.com", ".youtube.com", "www.youtube.com", "google.com", ".google.com", "accounts.google.com"}
	for _, domain := range cases {
		normalized, ok := normalizeDomain(strings.ToUpper(domain))
		if !ok {
			t.Errorf("expected %q to be accepted", domain)
			continue
		}
		if normalized != domain {
			t.Errorf("expected %q, got %q", domain, normalized)
		}
	}
}
