package client

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

// hostOnlyCookieJar keeps every cookie host-only (its Domain attribute is
// dropped), so a cookie goes back only to the host that set it, never to a
// sibling under the same domain. Hand-fix api-client-keeps-cookies.
func hostOnlyCookieJar() http.CookieJar {
	j, _ := cookiejar.New(nil)
	return hostOnlyJar{j}
}

type hostOnlyJar struct{ http.CookieJar }

func (j hostOnlyJar) SetCookies(u *url.URL, cs []*http.Cookie) {
	for _, c := range cs {
		c.Domain = ""
	}
	j.CookieJar.SetCookies(u, cs)
}
