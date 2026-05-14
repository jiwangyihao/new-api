package oauth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirstVerifiedPrimaryGitHubEmail(t *testing.T) {
	email := firstVerifiedPrimaryGitHubEmail([]gitHubEmail{
		{Email: "unverified@example.com", Primary: true, Verified: false},
		{Email: "secondary@example.com", Primary: false, Verified: true},
		{Email: "primary@example.com", Primary: true, Verified: true},
	})
	assert.Equal(t, "primary@example.com", email)
}

func TestFirstVerifiedPrimaryGitHubEmailReturnsEmptyWithoutVerifiedPrimary(t *testing.T) {
	email := firstVerifiedPrimaryGitHubEmail([]gitHubEmail{{Email: "nope@example.com", Primary: true, Verified: false}})
	assert.Equal(t, "", email)
}
