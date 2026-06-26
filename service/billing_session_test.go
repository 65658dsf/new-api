package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedBillingSessionUser(t *testing.T, id int, quota int) {
	t.Helper()
	user := &model.User{
		Id:       id,
		Username: "billing_session_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedBillingSessionToken(t *testing.T, id int, userId int, key string, remainQuota int) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "billing_session_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func seedBillingSessionSubscription(t *testing.T, id int, userId int, billingGroup string, amountTotal int64) {
	t.Helper()
	now := time.Now()
	plan := &model.SubscriptionPlan{
		Id:               id,
		Title:            "Billing Session Plan",
		PriceAmount:      0,
		Currency:         "USD",
		DurationUnit:     model.SubscriptionDurationMonth,
		DurationValue:    1,
		Enabled:          true,
		TotalAmount:      amountTotal,
		BillingGroup:     billingGroup,
		QuotaResetPeriod: model.SubscriptionResetNever,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	sub := &model.UserSubscription{
		Id:                  id,
		UserId:              userId,
		PlanId:              id,
		AmountTotal:         amountTotal,
		AmountUsed:          0,
		StartTime:           now.Unix(),
		EndTime:             now.Add(30 * 24 * time.Hour).Unix(),
		Status:              "active",
		AllowWalletOverflow: true,
		BillingGroup:        billingGroup,
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func makeBillingSessionRelayInfo(userId int, tokenId int, tokenKey string, usingGroup string, preference string, isPlayground ...bool) *relaycommon.RelayInfo {
	playground := true
	if len(isPlayground) > 0 {
		playground = isPlayground[0]
	}
	return &relaycommon.RelayInfo{
		UserId:          userId,
		TokenId:         tokenId,
		TokenKey:        tokenKey,
		UsingGroup:      usingGroup,
		OriginModelName: "gpt-test",
		RequestId:       "req-" + tokenKey + "-" + usingGroup,
		IsPlayground:    playground,
		UserSetting: dto.UserSetting{
			BillingPreference: preference,
		},
	}
}

func TestNewBillingSessionUsesSubscriptionOnlyForMatchingBillingGroup(t *testing.T) {
	truncate(t)
	c := &gin.Context{}

	const userId = 1001
	const tokenId = 1001
	const subId = 1001
	seedBillingSessionUser(t, userId, 5000)
	seedBillingSessionToken(t, tokenId, userId, "billing-match", 5000)
	seedBillingSessionSubscription(t, subId, userId, "vip", 10000)

	relayInfo := makeBillingSessionRelayInfo(userId, tokenId, "billing-match", "vip", "subscription_first")
	session, apiErr := NewBillingSession(c, relayInfo, 1000)

	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
	assert.Equal(t, subId, relayInfo.SubscriptionId)
	assert.Equal(t, 1000, relayInfo.FinalPreConsumedQuota)
	assert.Equal(t, 5000, getUserQuota(t, userId))
	assert.Equal(t, int64(1000), getSubscriptionUsed(t, subId))
}

func TestNewBillingSessionUsesWalletWhenBillingGroupDoesNotMatch(t *testing.T) {
	truncate(t)
	c := &gin.Context{}

	const userId = 1002
	const tokenId = 1002
	const subId = 1002
	seedBillingSessionUser(t, userId, 5000)
	seedBillingSessionToken(t, tokenId, userId, "billing-miss", 5000)
	seedBillingSessionSubscription(t, subId, userId, "vip", 10000)

	relayInfo := makeBillingSessionRelayInfo(userId, tokenId, "billing-miss", "default", "subscription_first")
	session, apiErr := NewBillingSession(c, relayInfo, 1000)

	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceWallet, relayInfo.BillingSource)
	assert.Zero(t, relayInfo.SubscriptionId)
	assert.Equal(t, 1000, relayInfo.FinalPreConsumedQuota)
	assert.Equal(t, 4000, getUserQuota(t, userId))
	assert.Equal(t, int64(0), getSubscriptionUsed(t, subId))
}

func TestNewBillingSessionUsesWalletForChatGPTWhenSubscriptionTargetsChatGPTPro(t *testing.T) {
	truncate(t)
	c := &gin.Context{}

	const userId = 1004
	const tokenId = 1004
	const subId = 1004
	seedBillingSessionUser(t, userId, 5000)
	seedBillingSessionToken(t, tokenId, userId, "billing-api-miss", 5000)
	seedBillingSessionSubscription(t, subId, userId, "ChatGPTPro", 10000)

	relayInfo := makeBillingSessionRelayInfo(userId, tokenId, "billing-api-miss", "ChatGPT", "subscription_first")
	session, apiErr := NewBillingSession(c, relayInfo, 1000)

	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceWallet, relayInfo.BillingSource)
	assert.Zero(t, relayInfo.SubscriptionId)
	assert.Equal(t, 1000, relayInfo.FinalPreConsumedQuota)
	assert.Equal(t, 4000, getUserQuota(t, userId))
	assert.Equal(t, int64(0), getSubscriptionUsed(t, subId))
}

func TestNewBillingSessionKeepsUngroupedSubscriptionGlobal(t *testing.T) {
	truncate(t)
	c := &gin.Context{}

	const userId = 1003
	const tokenId = 1003
	const subId = 1003
	seedBillingSessionUser(t, userId, 5000)
	seedBillingSessionToken(t, tokenId, userId, "billing-global", 5000)
	seedBillingSessionSubscription(t, subId, userId, "", 10000)

	relayInfo := makeBillingSessionRelayInfo(userId, tokenId, "billing-global", "default", "subscription_first")
	session, apiErr := NewBillingSession(c, relayInfo, 1000)

	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
	assert.Equal(t, subId, relayInfo.SubscriptionId)
	assert.Equal(t, 5000, getUserQuota(t, userId))
	assert.Equal(t, int64(1000), getSubscriptionUsed(t, subId))
}
