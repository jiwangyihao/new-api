package oauth

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type testBuiltInProvider struct{}

func (p *testBuiltInProvider) GetName() string { return "Built-in" }
func (p *testBuiltInProvider) IsEnabled() bool { return true }
func (p *testBuiltInProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	return nil, nil
}
func (p *testBuiltInProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	return nil, nil
}
func (p *testBuiltInProvider) IsUserIDTaken(providerUserID string) bool { return false }
func (p *testBuiltInProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	return nil
}
func (p *testBuiltInProvider) SetProviderUserID(user *model.User, providerUserID string) {}
func (p *testBuiltInProvider) GetProviderPrefix() string                                 { return "builtin_" }

func resetOAuthRegistryForTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	originalProviders := providers
	originalCustomProviderSlugs := customProviderSlugs
	providers = make(map[string]Provider)
	customProviderSlugs = make(map[string]bool)
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		providers = originalProviders
		customProviderSlugs = originalCustomProviderSlugs
		mu.Unlock()
	})
}

func TestRegisterCustomDoesNotOverrideBuiltInProvider(t *testing.T) {
	resetOAuthRegistryForTest(t)
	builtin := &testBuiltInProvider{}
	Register("wechat", builtin)
	RegisterCustom("wechat", NewGenericOAuthProvider(&model.CustomOAuthProvider{Id: 1, Name: "Custom WeChat", Slug: "wechat", Enabled: true}))

	assert.Same(t, builtin, GetProvider("wechat"))
	assert.False(t, IsCustomProvider("wechat"))
}

func TestUnregisterCustomProviderDoesNotRemoveBuiltInProvider(t *testing.T) {
	resetOAuthRegistryForTest(t)
	builtin := &testBuiltInProvider{}
	Register("wechat", builtin)

	UnregisterCustomProvider("wechat")

	assert.Same(t, builtin, GetProvider("wechat"))
	assert.False(t, IsCustomProvider("wechat"))
}

func TestLoadCustomProvidersSkipsBuiltInSlug(t *testing.T) {
	resetOAuthRegistryForTest(t)
	builtin := &testBuiltInProvider{}
	Register("wechat", builtin)
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:oauth-registry-test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.CustomOAuthProvider{}))
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.Create(&model.CustomOAuthProvider{Id: 1, Name: "Custom WeChat", Slug: "wechat", Enabled: true, ClientId: "client", ClientSecret: "secret", AuthorizationEndpoint: "https://example.com/auth", TokenEndpoint: "https://example.com/token", UserInfoEndpoint: "https://example.com/user"}).Error)

	require.NoError(t, LoadCustomProviders())

	assert.Same(t, builtin, GetProvider("wechat"))
	assert.False(t, IsCustomProvider("wechat"))
}
