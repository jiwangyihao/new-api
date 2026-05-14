package oauth

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func init() {
	Register("wechat", &WeChatProvider{})
}

// WeChatProvider implements the local WeChat verification-code OAuth bridge.
type WeChatProvider struct{}

func (p *WeChatProvider) GetName() string {
	return "WeChat"
}

func (p *WeChatProvider) IsEnabled() bool {
	return common.WeChatAuthEnabled
}

func (p *WeChatProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	return &OAuthToken{AccessToken: code}, nil
}

func (p *WeChatProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	return &OAuthUser{ProviderUserID: token.AccessToken}, nil
}

func (p *WeChatProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsWeChatIdAlreadyTaken(providerUserID)
}

func (p *WeChatProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.WeChatId = providerUserID
	return user.FillUserByWeChatId()
}

func (p *WeChatProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.WeChatId = providerUserID
}

func (p *WeChatProvider) GetProviderPrefix() string {
	return "wechat_"
}
