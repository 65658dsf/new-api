package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpAffiliateRewardTest(t *testing.T) {
	t.Helper()
	truncateTables(t)

	oldQuotaPerUnit := common.QuotaPerUnit
	oldQuotaForNewUser := common.QuotaForNewUser
	oldQuotaForInviter := common.QuotaForInviter
	oldQuotaForInvitee := common.QuotaForInvitee
	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	oldQuotaSetting := operation_setting.GetQuotaSetting()
	paymentSetting := operation_setting.GetPaymentSetting()
	oldComplianceConfirmed := paymentSetting.ComplianceConfirmed
	oldComplianceTermsVersion := paymentSetting.ComplianceTermsVersion

	common.QuotaPerUnit = 1
	common.QuotaForNewUser = 0
	common.QuotaForInviter = 0
	common.QuotaForInvitee = 0
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	setInviterRewardSetting(t, operation_setting.InviterRewardModePercentage, 1)
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		common.QuotaForNewUser = oldQuotaForNewUser
		common.QuotaForInviter = oldQuotaForInviter
		common.QuotaForInvitee = oldQuotaForInvitee
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		rewardJSON, err := common.Marshal(oldQuotaSetting.InviterReward)
		require.NoError(t, err)
		require.NoError(t, operation_setting.UpdateQuotaSetting(map[string]string{
			"enable_free_model_pre_consume": strconv.FormatBool(oldQuotaSetting.EnableFreeModelPreConsume),
			"inviter_reward":                string(rewardJSON),
		}))
		paymentSetting.ComplianceConfirmed = oldComplianceConfirmed
		paymentSetting.ComplianceTermsVersion = oldComplianceTermsVersion
	})
}

func setInviterRewardSetting(t *testing.T, mode string, percentage float64) {
	t.Helper()
	rewardJSON, err := common.Marshal(operation_setting.InviterRewardSetting{
		Mode:       mode,
		Percentage: percentage,
	})
	require.NoError(t, err)
	require.NoError(t, operation_setting.UpdateQuotaSetting(map[string]string{
		"inviter_reward": string(rewardJSON),
	}))
}

func seedTopUpAffiliateRewardUsers(t *testing.T) (User, User) {
	t.Helper()
	inviter := User{
		Id:       8101,
		Username: "affiliate-inviter",
		AffCode:  "affinviter",
		Status:   common.UserStatusEnabled,
	}
	invitee := User{
		Id:        8102,
		Username:  "affiliate-invitee",
		AffCode:   "affinvitee",
		InviterId: inviter.Id,
		Status:    common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	return inviter, invitee
}

func TestCompleteEpayTopUpAwardsExtraInviterPercentageExactlyOnce(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	inviter, invitee := seedTopUpAffiliateRewardUsers(t)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          invitee.Id,
		Amount:          100,
		Money:           100,
		TradeNo:         "affiliate-epay-order",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}).Error)

	completedTopUp, quota, completed, err := CompleteEpayTopUp("affiliate-epay-order", "alipay")
	require.NoError(t, err)
	require.True(t, completed)
	require.NotNil(t, completedTopUp)
	assert.Equal(t, 100, quota)

	var actualInviter User
	var actualInvitee User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, 1, actualInviter.AffQuota)
	assert.Equal(t, 1, actualInviter.AffHistoryQuota)
	assert.Equal(t, 100, actualInvitee.Quota)

	_, quota, completed, err = CompleteEpayTopUp("affiliate-epay-order", "alipay")
	require.NoError(t, err)
	assert.False(t, completed)
	assert.Zero(t, quota)
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, 1, actualInviter.AffQuota)
	assert.Equal(t, 1, actualInviter.AffHistoryQuota)
	assert.Equal(t, 100, actualInvitee.Quota)
}

func TestStripeTopUpReplayDoesNotDuplicateQuotaOrInviterReward(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	inviter, invitee := seedTopUpAffiliateRewardUsers(t)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          invitee.Id,
		Amount:          100,
		Money:           100,
		TradeNo:         "affiliate-stripe-order",
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusPending,
	}).Error)

	require.NoError(t, Recharge("affiliate-stripe-order", "cus_affiliate", "127.0.0.1"))
	require.NoError(t, Recharge("affiliate-stripe-order", "cus_affiliate", "127.0.0.1"))

	var actualInviter User
	var actualInvitee User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, 1, actualInviter.AffQuota)
	assert.Equal(t, 1, actualInviter.AffHistoryQuota)
	assert.Equal(t, 100, actualInvitee.Quota)
}

func TestCreemTopUpReplayDoesNotDuplicateQuotaOrInviterReward(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	inviter, invitee := seedTopUpAffiliateRewardUsers(t)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          invitee.Id,
		Amount:          100,
		Money:           10,
		TradeNo:         "affiliate-creem-order",
		PaymentMethod:   PaymentMethodCreem,
		PaymentProvider: PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
	}).Error)

	require.NoError(t, RechargeCreem("affiliate-creem-order", "", "", "127.0.0.1"))
	require.NoError(t, RechargeCreem("affiliate-creem-order", "", "", "127.0.0.1"))

	var actualInviter User
	var actualInvitee User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, 1, actualInviter.AffQuota)
	assert.Equal(t, 1, actualInviter.AffHistoryQuota)
	assert.Equal(t, 100, actualInvitee.Quota)
}

func TestWaffoTopUpReplaysDoNotDuplicateQuotaOrInviterReward(t *testing.T) {
	tests := []struct {
		name            string
		paymentMethod   string
		paymentProvider string
		recharge        func(string) error
	}{
		{
			name:            "waffo",
			paymentMethod:   PaymentMethodWaffo,
			paymentProvider: PaymentProviderWaffo,
			recharge: func(tradeNo string) error {
				return RechargeWaffo(tradeNo, "127.0.0.1")
			},
		},
		{
			name:            "waffo pancake",
			paymentMethod:   PaymentMethodWaffoPancake,
			paymentProvider: PaymentProviderWaffoPancake,
			recharge:        RechargeWaffoPancake,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupTopUpAffiliateRewardTest(t)
			inviter, invitee := seedTopUpAffiliateRewardUsers(t)
			tradeNo := "affiliate-" + test.paymentProvider + "-order"
			require.NoError(t, DB.Create(&TopUp{
				UserId:          invitee.Id,
				Amount:          100,
				Money:           100,
				TradeNo:         tradeNo,
				PaymentMethod:   test.paymentMethod,
				PaymentProvider: test.paymentProvider,
				Status:          common.TopUpStatusPending,
			}).Error)

			require.NoError(t, test.recharge(tradeNo))
			require.NoError(t, test.recharge(tradeNo))

			var actualInviter User
			var actualInvitee User
			require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
			require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
			assert.Equal(t, 1, actualInviter.AffQuota)
			assert.Equal(t, 1, actualInviter.AffHistoryQuota)
			assert.Equal(t, 100, actualInvitee.Quota)
		})
	}
}

func TestInviterRewardSaturatesWithoutBlockingInviteeTopUp(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	inviter, invitee := seedTopUpAffiliateRewardUsers(t)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", inviter.Id).Updates(map[string]interface{}{
		"aff_quota":   common.MaxQuota - 1,
		"aff_history": common.MaxQuota - 1,
	}).Error)

	for _, tradeNo := range []string{"affiliate-saturation-1", "affiliate-saturation-2"} {
		require.NoError(t, DB.Create(&TopUp{
			UserId:          invitee.Id,
			Amount:          100,
			Money:           100,
			TradeNo:         tradeNo,
			PaymentMethod:   "alipay",
			PaymentProvider: PaymentProviderEpay,
			Status:          common.TopUpStatusPending,
		}).Error)
		_, _, completed, err := CompleteEpayTopUp(tradeNo, "alipay")
		require.NoError(t, err)
		assert.True(t, completed)
	}

	var actualInviter User
	var actualInvitee User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, common.MaxQuota, actualInviter.AffQuota)
	assert.Equal(t, common.MaxQuota, actualInviter.AffHistoryQuota)
	assert.Equal(t, 200, actualInvitee.Quota)
}

func TestFixedInviterRewardSaturatesWithoutChangingInviteeReward(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	common.QuotaForNewUser = 10
	common.QuotaForInvitee = 20
	common.QuotaForInviter = 100
	setInviterRewardSetting(t, operation_setting.InviterRewardModeFixed, 0)

	inviter := User{
		Id:              8151,
		Username:        "fixed-saturation-inviter",
		AffCode:         "fixed-saturation-inviter",
		AffQuota:        common.MaxQuota - 1,
		AffHistoryQuota: common.MaxQuota - 1,
		Status:          common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := &User{Username: "fixed-saturation-invitee", Status: common.UserStatusEnabled}

	require.NoError(t, invitee.Insert(inviter.Id))

	var actualInviter User
	var actualInvitee User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, common.MaxQuota, actualInviter.AffQuota)
	assert.Equal(t, common.MaxQuota, actualInviter.AffHistoryQuota)
	assert.Equal(t, 1, actualInviter.AffCount)
	assert.Equal(t, 30, actualInvitee.Quota)
}

func TestManualCompleteTopUpUsesCreemCreditedQuotaForExtraInviterReward(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	common.QuotaPerUnit = 10
	inviter, invitee := seedTopUpAffiliateRewardUsers(t)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          invitee.Id,
		Amount:          100,
		Money:           10,
		TradeNo:         "affiliate-creem-admin-order",
		PaymentMethod:   PaymentMethodCreem,
		PaymentProvider: PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
	}).Error)

	require.NoError(t, ManualCompleteTopUp("affiliate-creem-admin-order", "127.0.0.1"))

	var actualInviter User
	var actualInvitee User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, 1, actualInviter.AffQuota)
	assert.Equal(t, 1, actualInviter.AffHistoryQuota)
	assert.Equal(t, 100, actualInvitee.Quota)
}

func TestManualCompleteSubscriptionTopUpDoesNotAwardInviterPercentage(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	inviter, invitee := seedTopUpAffiliateRewardUsers(t)
	const tradeNo = "affiliate-subscription-order"
	require.NoError(t, DB.Create(&TopUp{
		UserId:          invitee.Id,
		Amount:          100,
		Money:           100,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}).Error)
	require.NoError(t, DB.Create(&SubscriptionOrder{
		UserId:          invitee.Id,
		PlanId:          1,
		Money:           100,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}).Error)

	require.NoError(t, ManualCompleteTopUp(tradeNo, "127.0.0.1"))

	var actualInviter User
	require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
	assert.Zero(t, actualInviter.AffQuota)
	assert.Zero(t, actualInviter.AffHistoryQuota)
}

func TestInvitationRewardModesKeepInviteeRewardBehavior(t *testing.T) {
	t.Run("fixed", func(t *testing.T) {
		setupTopUpAffiliateRewardTest(t)
		common.QuotaForNewUser = 10
		common.QuotaForInvitee = 20
		common.QuotaForInviter = 30
		setInviterRewardSetting(t, operation_setting.InviterRewardModeFixed, 1)

		inviter := User{Id: 8201, Username: "fixed-inviter", AffCode: "fixed-inviter", Status: common.UserStatusEnabled}
		require.NoError(t, DB.Create(&inviter).Error)
		invitee := &User{Username: "fixed-invitee", Status: common.UserStatusEnabled}
		require.NoError(t, invitee.Insert(inviter.Id))

		var actualInviter User
		var actualInvitee User
		require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
		require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
		assert.Equal(t, 30, actualInviter.AffQuota)
		assert.Equal(t, 30, actualInviter.AffHistoryQuota)
		assert.Equal(t, 1, actualInviter.AffCount)
		assert.Equal(t, 30, actualInvitee.Quota)
		assert.Equal(t, inviter.Id, actualInvitee.InviterId)
	})

	t.Run("percentage", func(t *testing.T) {
		setupTopUpAffiliateRewardTest(t)
		common.QuotaForNewUser = 10
		common.QuotaForInvitee = 20
		common.QuotaForInviter = 30
		setInviterRewardSetting(t, operation_setting.InviterRewardModePercentage, 1)

		inviter := User{Id: 8301, Username: "percentage-inviter", AffCode: "percentage-inviter", Status: common.UserStatusEnabled}
		require.NoError(t, DB.Create(&inviter).Error)
		invitee := &User{Username: "percentage-invitee", Status: common.UserStatusEnabled}
		require.NoError(t, invitee.Insert(inviter.Id))

		var actualInviter User
		var actualInvitee User
		require.NoError(t, DB.First(&actualInviter, inviter.Id).Error)
		require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
		assert.Zero(t, actualInviter.AffQuota)
		assert.Zero(t, actualInviter.AffHistoryQuota)
		assert.Equal(t, 1, actualInviter.AffCount)
		assert.Equal(t, 30, actualInvitee.Quota)
		assert.Equal(t, inviter.Id, actualInvitee.InviterId)
	})
}

func TestInsertWithTxPersistsInviterID(t *testing.T) {
	setupTopUpAffiliateRewardTest(t)
	inviter := User{Id: 8401, Username: "oauth-inviter", AffCode: "oauth-inviter", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := &User{Username: "oauth-invitee", Status: common.UserStatusEnabled}

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return invitee.InsertWithTx(tx, inviter.Id)
	}))

	var actualInvitee User
	require.NoError(t, DB.First(&actualInvitee, invitee.Id).Error)
	assert.Equal(t, inviter.Id, actualInvitee.InviterId)
}
