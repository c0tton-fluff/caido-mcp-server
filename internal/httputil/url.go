package httputil

import "fmt"

func BuildURL(
	isTLS bool, host string, port int, path, query string,
) string {
	scheme := "http"
	if isTLS {
		scheme = "https"
	}
	u := fmt.Sprintf("%s://%s", scheme, host)
	if (isTLS && port != 443) || (!isTLS && port != 80) {
		u = fmt.Sprintf("%s:%d", u, port)
	}
	u += path
	if query != "" {
		u += "?" + query
	}
	return u
}
