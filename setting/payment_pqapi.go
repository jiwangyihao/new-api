package setting

import (
	"errors"
	"net/url"
	"strings"
)

const PQAPIDefaultBaseURL = "https://shop.pqapi.shop"

var PQAPIBaseURL = PQAPIDefaultBaseURL
var PQAPISiteID = ""
var PQAPISecret = ""

func NormalizePQAPIBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("PQAPI base URL must be a valid HTTPS origin")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("PQAPI base URL must not contain credentials, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func ApplyPQAPIRuntimeOption(key, value string) error {
	value = strings.TrimSpace(value)
	switch key {
	case "PQAPIBaseURL":
		normalized, err := NormalizePQAPIBaseURL(value)
		if err != nil {
			return err
		}
		PQAPIBaseURL = normalized
	case "PQAPISiteID":
		PQAPISiteID = value
	case "PQAPISecret":
		// An empty form value never erases an existing production secret.
		if value != "" {
			PQAPISecret = value
		}
	}
	return nil
}
