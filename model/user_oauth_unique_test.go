package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserOAuthUniqueIndexesAllowBlankValues(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{Id: 79001, Username: "blank_oauth_a", Status: 1, AffCode: "blankA"}).Error)
	require.NoError(t, DB.Create(&User{Id: 79002, Username: "blank_oauth_b", Status: 1, AffCode: "blankB"}).Error)
}
