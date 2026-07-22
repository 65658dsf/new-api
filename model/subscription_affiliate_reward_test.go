package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompleteSubscriptionOrderAwardsExtraInviterPercentageExactlyOnce(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	inviter, invitee := seedTopUpAffiliateRewardUsers(t)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Update("quota", 37).Error)

	plan := SubscriptionPlan{
		Id:               8501,
		Title:            "Affiliate subscription",
		PriceAmount:      200,
		Currency:         "USD",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		Enabled:          true,
		TotalAmount:      5000,
		QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&plan).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		UserId:          invitee.Id,
		PlanId:          plan.Id,
		Money:           100,
		TradeNo:         "affiliate-subscription-payment",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}).Error)

	for range 2 {
		require.NoError(t, CompleteSubscriptionOrder(
			"affiliate-subscription-payment",
			`{"provider":"epay"}`,
			PaymentProviderEpay,
			"alipay",
		))
	}

	var actualInviter User
	var actualInvitee User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, 1, actualInviter.AffQuota)
	assert.Equal(t, 1, actualInviter.AffHistoryQuota)
	assert.Equal(t, 37, actualInvitee.Quota)

	var subscriptions []UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", invitee.Id, plan.Id).Find(&subscriptions).Error)
	require.Len(t, subscriptions, 1)
	assert.Equal(t, int64(5000), subscriptions[0].AmountTotal)
	assert.Zero(t, subscriptions[0].AmountUsed)
	assert.Equal(t, "active", subscriptions[0].Status)
	assert.Equal(t, "order", subscriptions[0].Source)

	var order SubscriptionOrder
	require.NoError(t, DB.Where("trade_no = ?", "affiliate-subscription-payment").First(&order).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)

	var shadowTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", order.TradeNo).First(&shadowTopUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, shadowTopUp.Status)
	assert.Zero(t, shadowTopUp.Amount)
}

func TestPurchaseSubscriptionWithBalanceDoesNotRewardInviterAgain(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	inviter, invitee := seedTopUpAffiliateRewardUsers(t)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Update("quota", 150).Error)

	plan := SubscriptionPlan{
		Id:               8502,
		Title:            "Balance subscription",
		PriceAmount:      100,
		Currency:         "USD",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		Enabled:          true,
		TotalAmount:      5000,
		QuotaResetPeriod: SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(&plan).Error)

	require.NoError(t, PurchaseSubscriptionWithBalance(invitee.Id, plan.Id))

	var actualInviter User
	var actualInvitee User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Zero(t, actualInviter.AffQuota)
	assert.Zero(t, actualInviter.AffHistoryQuota)
	assert.Equal(t, 50, actualInvitee.Quota)

	var subscriptionCount int64
	require.NoError(t, DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", invitee.Id, plan.Id).
		Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)

	var order SubscriptionOrder
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", invitee.Id, plan.Id).First(&order).Error)
	assert.Equal(t, PaymentProviderBalance, order.PaymentProvider)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
}
