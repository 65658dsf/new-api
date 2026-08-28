package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedSubscriptionResetPlan(t *testing.T, plan *SubscriptionPlan) {
	t.Helper()
	require.NoError(t, DB.Create(plan).Error)
}

func seedSubscriptionResetSub(t *testing.T, sub *UserSubscription) {
	t.Helper()
	require.NoError(t, DB.Create(sub).Error)
}

func getSubscriptionResetSub(t *testing.T, id int) UserSubscription {
	t.Helper()
	var sub UserSubscription
	require.NoError(t, DB.Where("id = ?", id).First(&sub).Error)
	return sub
}

func TestAdminResetUserSubscriptionsByPlanResetsAllActiveMatchesAndAdvancesTime(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:               9101,
		Title:            "Pro",
		PriceAmount:      10,
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      1000,
		QuotaResetPeriod: SubscriptionResetDaily,
	}
	otherPlan := &SubscriptionPlan{
		Id:               9102,
		Title:            "Basic",
		PriceAmount:      1,
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      100,
		QuotaResetPeriod: SubscriptionResetDaily,
	}
	seedSubscriptionResetPlan(t, plan)
	seedSubscriptionResetPlan(t, otherPlan)

	activeEnd := now + 30*24*3600
	expiredEnd := now - 1
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9201, UserId: 101, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 300, StartTime: now - 3600, EndTime: activeEnd, Status: "active", LastResetTime: now - 3600, NextResetTime: now + 120})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9202, UserId: 101, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 500, StartTime: now - 3600, EndTime: activeEnd, Status: "active", LastResetTime: now - 3600, NextResetTime: now + 120})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9203, UserId: 101, PlanId: otherPlan.Id, AmountTotal: 100, AmountUsed: 60, StartTime: now - 3600, EndTime: activeEnd, Status: "active", LastResetTime: now - 3600, NextResetTime: now + 120})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9204, UserId: 101, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 700, StartTime: now - 7200, EndTime: expiredEnd, Status: "active", LastResetTime: now - 3600, NextResetTime: now - 10})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9205, UserId: 102, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 800, StartTime: now - 3600, EndTime: activeEnd, Status: "active", LastResetTime: now - 3600, NextResetTime: now + 120})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9206, UserId: 101, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 900, StartTime: now - 3600, EndTime: activeEnd, Status: "cancelled", LastResetTime: now - 3600, NextResetTime: now + 120})

	beforeReset := GetDBTimestamp()
	result, err := AdminResetUserSubscriptionsByPlan(101, plan.Id, true)
	afterReset := GetDBTimestamp()

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, plan.Id, result.PlanId)
	assert.Equal(t, 2, result.MatchedCount)
	assert.Equal(t, 2, result.ResetCount)
	assert.Equal(t, 1, result.UserCount)
	assert.Equal(t, []int{101}, result.AffectedUserIds)
	assert.True(t, result.AdvanceResetTime)

	for _, id := range []int{9201, 9202} {
		sub := getSubscriptionResetSub(t, id)
		assert.Zero(t, sub.AmountUsed)
		assert.GreaterOrEqual(t, sub.LastResetTime, beforeReset)
		assert.LessOrEqual(t, sub.LastResetTime, afterReset)
		assert.Equal(t, calcNextResetTime(time.Unix(sub.LastResetTime, 0), plan, sub.EndTime), sub.NextResetTime)
	}
	assert.EqualValues(t, 60, getSubscriptionResetSub(t, 9203).AmountUsed)
	assert.EqualValues(t, 700, getSubscriptionResetSub(t, 9204).AmountUsed)
	assert.EqualValues(t, 800, getSubscriptionResetSub(t, 9205).AmountUsed)
	assert.EqualValues(t, 900, getSubscriptionResetSub(t, 9206).AmountUsed)
}

func TestAdminResetUserSubscriptionsByPlanKeepsResetTimes(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:               9301,
		Title:            "Team",
		PriceAmount:      20,
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      2000,
		QuotaResetPeriod: SubscriptionResetMonthly,
	}
	seedSubscriptionResetPlan(t, plan)

	lastReset := now - 86400
	nextReset := now + 86400
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9302, UserId: 201, PlanId: plan.Id, AmountTotal: 2000, AmountUsed: 1200, StartTime: now - 172800, EndTime: now + 30*24*3600, Status: "active", LastResetTime: lastReset, NextResetTime: nextReset})

	result, err := AdminResetUserSubscriptionsByPlan(201, plan.Id, false)

	require.NoError(t, err)
	assert.False(t, result.AdvanceResetTime)
	sub := getSubscriptionResetSub(t, 9302)
	assert.Zero(t, sub.AmountUsed)
	assert.Equal(t, lastReset, sub.LastResetTime)
	assert.Equal(t, nextReset, sub.NextResetTime)
}

func TestAdminResetUserSubscriptionsByPlanNoActiveMatchReturnsError(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:            9401,
		Title:         "Expired",
		PriceAmount:   10,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   1000,
	}
	seedSubscriptionResetPlan(t, plan)
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9402, UserId: 301, PlanId: plan.Id, AmountTotal: 1000, AmountUsed: 500, StartTime: now - 7200, EndTime: now - 1, Status: "active"})

	result, err := AdminResetUserSubscriptionsByPlan(301, plan.Id, true)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, strings.Contains(err.Error(), "该用户没有有效的此套餐订阅"))
}

func TestAdminResetPlanSubscriptionsResetsAllActiveUsers(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:               9501,
		Title:            "Business",
		PriceAmount:      30,
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      3000,
		QuotaResetPeriod: SubscriptionResetNever,
	}
	seedSubscriptionResetPlan(t, plan)

	activeEnd := now + 30*24*3600
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9502, UserId: 401, PlanId: plan.Id, AmountTotal: 3000, AmountUsed: 1000, StartTime: now - 3600, EndTime: activeEnd, Status: "active", LastResetTime: now - 3600, NextResetTime: now + 10})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9503, UserId: 401, PlanId: plan.Id, AmountTotal: 3000, AmountUsed: 1100, StartTime: now - 3500, EndTime: activeEnd, Status: "active", LastResetTime: now - 3600, NextResetTime: now + 10})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9504, UserId: 402, PlanId: plan.Id, AmountTotal: 3000, AmountUsed: 1200, StartTime: now - 3400, EndTime: activeEnd, Status: "active", LastResetTime: now - 3600, NextResetTime: now + 10})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9505, UserId: 403, PlanId: plan.Id, AmountTotal: 3000, AmountUsed: 1300, StartTime: now - 7200, EndTime: now - 1, Status: "active", LastResetTime: now - 3600, NextResetTime: now - 10})
	seedSubscriptionResetSub(t, &UserSubscription{Id: 9506, UserId: 404, PlanId: plan.Id, AmountTotal: 3000, AmountUsed: 1400, StartTime: now - 3600, EndTime: activeEnd, Status: "cancelled", LastResetTime: now - 3600, NextResetTime: now + 10})

	result, err := AdminResetPlanSubscriptions(plan.Id, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 3, result.MatchedCount)
	assert.Equal(t, 3, result.ResetCount)
	assert.Equal(t, 2, result.UserCount)
	assert.Equal(t, []int{401, 402}, result.AffectedUserIds)
	for _, id := range []int{9502, 9503, 9504} {
		sub := getSubscriptionResetSub(t, id)
		assert.Zero(t, sub.AmountUsed)
		assert.Zero(t, sub.LastResetTime)
		assert.Zero(t, sub.NextResetTime)
	}
	assert.EqualValues(t, 1300, getSubscriptionResetSub(t, 9505).AmountUsed)
	assert.EqualValues(t, 1400, getSubscriptionResetSub(t, 9506).AmountUsed)
}

func TestAdminResetPlanSubscriptionsNoMatchSucceeds(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id:            9601,
		Title:         "Empty",
		PriceAmount:   10,
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   1000,
	}
	seedSubscriptionResetPlan(t, plan)

	result, err := AdminResetPlanSubscriptions(plan.Id, true)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Zero(t, result.MatchedCount)
	assert.Zero(t, result.ResetCount)
	assert.Zero(t, result.UserCount)
	assert.Empty(t, result.AffectedUserIds)
}

func TestCreateUserSubscriptionSnapshotsQuotaResetSettings(t *testing.T) {
	truncateTables(t)

	plan := &SubscriptionPlan{
		Id:                      9701,
		Title:                   "Snapshot",
		DurationUnit:            SubscriptionDurationDay,
		DurationValue:           2,
		TotalAmount:             1000,
		QuotaResetPeriod:        SubscriptionResetCustom,
		QuotaResetCustomSeconds: 12345,
	}
	user := &User{Id: 9702, Username: "snapshot-user", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(user).Error)

	sub, err := CreateUserSubscriptionFromPlanTx(DB, user.Id, plan, "test")
	require.NoError(t, err)
	require.NotNil(t, sub)
	assert.Equal(t, SubscriptionResetCustom, sub.QuotaResetPeriod)
	assert.EqualValues(t, 12345, sub.QuotaResetCustomSeconds)
}

func TestAdminResetUserSubscriptionUsesInstanceResetSnapshot(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:               9801,
		Title:            "Plan",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		QuotaResetPeriod: SubscriptionResetDaily,
		TotalAmount:      1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	sub := &UserSubscription{
		Id:                      9802,
		UserId:                  9803,
		PlanId:                  plan.Id,
		AmountTotal:             1000,
		AmountUsed:              700,
		StartTime:               now - 3600,
		EndTime:                 now + 3*24*3600,
		Status:                  "active",
		QuotaResetPeriod:        SubscriptionResetCustom,
		QuotaResetCustomSeconds: 12345,
	}
	require.NoError(t, DB.Create(sub).Error)

	// Changing the plan after issuance must not change this instance's reset.
	plan.QuotaResetPeriod = SubscriptionResetNever
	require.NoError(t, DB.Model(plan).Updates(map[string]interface{}{
		"quota_reset_period": SubscriptionResetNever,
	}).Error)

	before := GetDBTimestamp()
	updated, err := AdminResetUserSubscription(sub.Id, true)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Zero(t, updated.AmountUsed)
	assert.Equal(t, SubscriptionResetCustom, updated.QuotaResetPeriod)
	assert.EqualValues(t, 12345, updated.QuotaResetCustomSeconds)
	assert.GreaterOrEqual(t, updated.NextResetTime, before+12345)
	assert.LessOrEqual(t, updated.NextResetTime, before+12346)
}

func TestAdminUpdateUserSubscriptionChangesInstanceOnly(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:               9901,
		Title:            "Original",
		DurationUnit:     SubscriptionDurationMonth,
		DurationValue:    1,
		TotalAmount:      1000,
		QuotaResetPeriod: SubscriptionResetDaily,
	}
	require.NoError(t, DB.Create(plan).Error)
	sub := &UserSubscription{
		Id:               9902,
		UserId:           9903,
		PlanId:           plan.Id,
		AmountTotal:      1000,
		AmountUsed:       100,
		StartTime:        now - 100,
		EndTime:          now + 24*3600,
		Status:           "active",
		BillingGroup:     "",
		QuotaResetPeriod: SubscriptionResetDaily,
	}
	require.NoError(t, DB.Create(sub).Error)

	amountTotal := int64(2000)
	endTime := now + 3*24*3600
	status := "active"
	allowOverflow := false
	billingGroup := "vip"
	downgradeGroup := "default"
	period := SubscriptionResetCustom
	customSeconds := int64(3600)
	updated, err := AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{
		AmountTotal:             &amountTotal,
		EndTime:                 &endTime,
		Status:                  &status,
		AllowWalletOverflow:     &allowOverflow,
		BillingGroup:            &billingGroup,
		DowngradeGroup:          &downgradeGroup,
		QuotaResetPeriod:        &period,
		QuotaResetCustomSeconds: &customSeconds,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.EqualValues(t, amountTotal, updated.AmountTotal)
	assert.EqualValues(t, endTime, updated.EndTime)
	assert.False(t, updated.AllowWalletOverflow)
	assert.Equal(t, billingGroup, updated.BillingGroup)
	assert.Equal(t, downgradeGroup, updated.DowngradeGroup)
	assert.Equal(t, period, updated.QuotaResetPeriod)
	assert.EqualValues(t, customSeconds, updated.QuotaResetCustomSeconds)

	var storedPlan SubscriptionPlan
	require.NoError(t, DB.First(&storedPlan, plan.Id).Error)
	assert.EqualValues(t, 1000, storedPlan.TotalAmount)
	assert.Equal(t, SubscriptionResetDaily, storedPlan.QuotaResetPeriod)

	negative := int64(-1)
	_, err = AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{AmountTotal: &negative})
	require.Error(t, err)
	var storedSub UserSubscription
	require.NoError(t, DB.First(&storedSub, sub.Id).Error)
	assert.EqualValues(t, amountTotal, storedSub.AmountTotal)
}

func TestAdminUpdateUserSubscriptionKeepsResetScheduleWhenEffectiveSettingsUnchanged(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:            9911,
		Title:         "Schedule plan",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
	}
	require.NoError(t, DB.Create(plan).Error)
	startTime := now - 3600
	endTime := now + 24*3600
	lastResetTime := now - 120
	nextResetTime := now + 1800
	sub := &UserSubscription{
		Id:                      9912,
		UserId:                  9913,
		PlanId:                  plan.Id,
		AmountTotal:             1000,
		AmountUsed:              200,
		StartTime:               startTime,
		EndTime:                 endTime,
		Status:                  "active",
		QuotaResetPeriod:        SubscriptionResetCustom,
		QuotaResetCustomSeconds: 3600,
		LastResetTime:           lastResetTime,
		NextResetTime:           nextResetTime,
	}
	require.NoError(t, DB.Create(sub).Error)

	amountTotal := int64(2000)
	period := SubscriptionResetCustom
	customSeconds := int64(3600)
	updated, err := AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{
		AmountTotal:             &amountTotal,
		StartTime:               &startTime,
		EndTime:                 &endTime,
		QuotaResetPeriod:        &period,
		QuotaResetCustomSeconds: &customSeconds,
	})
	require.NoError(t, err)
	assert.EqualValues(t, amountTotal, updated.AmountTotal)
	assert.EqualValues(t, lastResetTime, updated.LastResetTime)
	assert.EqualValues(t, nextResetTime, updated.NextResetTime)
}

func TestAdminUpdateUserSubscriptionSnapshotsLegacyResetSettingsWithoutChangingSchedule(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{
		Id:                      9914,
		Title:                   "Legacy snapshot plan",
		QuotaResetPeriod:        SubscriptionResetCustom,
		QuotaResetCustomSeconds: 7200,
	}
	require.NoError(t, DB.Create(plan).Error)
	lastResetTime := now - 300
	nextResetTime := now + 900
	sub := &UserSubscription{
		Id:            9915,
		UserId:        9916,
		PlanId:        plan.Id,
		AmountTotal:   1000,
		AmountUsed:    200,
		StartTime:     now - 3600,
		EndTime:       now + 24*3600,
		Status:        "active",
		LastResetTime: lastResetTime,
		NextResetTime: nextResetTime,
		BillingGroup:  "default",
	}
	require.NoError(t, DB.Create(sub).Error)

	billingGroup := "vip"
	updated, err := AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{
		BillingGroup: &billingGroup,
	})
	require.NoError(t, err)
	assert.Equal(t, SubscriptionResetCustom, updated.QuotaResetPeriod)
	assert.EqualValues(t, 7200, updated.QuotaResetCustomSeconds)
	assert.EqualValues(t, lastResetTime, updated.LastResetTime)
	assert.EqualValues(t, nextResetTime, updated.NextResetTime)

	require.NoError(t, DB.Model(plan).Updates(map[string]interface{}{
		"quota_reset_period":         SubscriptionResetDaily,
		"quota_reset_custom_seconds": 0,
	}).Error)
	var updatedPlan SubscriptionPlan
	require.NoError(t, DB.First(&updatedPlan, plan.Id).Error)
	assert.Equal(t, SubscriptionResetDaily, updatedPlan.QuotaResetPeriod)
	stored := getSubscriptionResetSub(t, sub.Id)
	period, customSeconds := subscriptionResetSettings(&stored, &updatedPlan)
	assert.Equal(t, SubscriptionResetCustom, period)
	assert.EqualValues(t, 7200, customSeconds)
}

func TestAdminUpdateUserSubscriptionRecalculatesResetScheduleWhenCustomSecondsChange(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{Id: 9921, Title: "Custom schedule"}
	require.NoError(t, DB.Create(plan).Error)
	sub := &UserSubscription{
		Id:                      9922,
		UserId:                  9923,
		PlanId:                  plan.Id,
		AmountTotal:             1000,
		StartTime:               now - 60,
		EndTime:                 now + 24*3600,
		Status:                  "active",
		QuotaResetPeriod:        SubscriptionResetCustom,
		QuotaResetCustomSeconds: 3600,
		LastResetTime:           now - 3600,
		NextResetTime:           now + 60,
	}
	require.NoError(t, DB.Create(sub).Error)

	customSeconds := int64(7200)
	before := GetDBTimestamp()
	updated, err := AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{
		QuotaResetCustomSeconds: &customSeconds,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, updated.NextResetTime, before+7200)
	assert.LessOrEqual(t, updated.NextResetTime, before+7201)
	assert.EqualValues(t, before, updated.LastResetTime)
}

func TestAdminUpdateUserSubscriptionReactivationRecalculatesResetSchedule(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{Id: 9924, Title: "Reactivation schedule"}
	require.NoError(t, DB.Create(plan).Error)
	sub := &UserSubscription{
		Id:                      9925,
		UserId:                  9926,
		PlanId:                  plan.Id,
		AmountTotal:             1000,
		AmountUsed:              250,
		StartTime:               now - 7200,
		EndTime:                 now + 24*3600,
		Status:                  "expired",
		QuotaResetPeriod:        SubscriptionResetCustom,
		QuotaResetCustomSeconds: 3600,
		LastResetTime:           now - 7200,
		NextResetTime:           0,
	}
	require.NoError(t, DB.Create(sub).Error)

	status := "active"
	before := GetDBTimestamp()
	updated, err := AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{Status: &status})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, updated.NextResetTime, before+3600)
	assert.LessOrEqual(t, updated.NextResetTime, before+3601)
	assert.GreaterOrEqual(t, updated.LastResetTime, before)
	assert.LessOrEqual(t, updated.LastResetTime, before+1)
}

func TestAdminUpdateUserSubscriptionSyncsUpgradeGroupTransitions(t *testing.T) {
	t.Run("active upgrade group replacement preserves original fallback", func(t *testing.T) {
		truncateTables(t)
		now := GetDBTimestamp()
		user := &User{Id: 9931, Username: "subscription-group-replace", Group: "pro", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
		plan := &SubscriptionPlan{Id: 9932, Title: "Group replace"}
		require.NoError(t, DB.Create(user).Error)
		require.NoError(t, DB.Create(plan).Error)
		sub := &UserSubscription{
			Id:            9933,
			UserId:        user.Id,
			PlanId:        plan.Id,
			StartTime:     now - 60,
			EndTime:       now + 3600,
			Status:        "active",
			UpgradeGroup:  "pro",
			PrevUserGroup: "default",
		}
		require.NoError(t, DB.Create(sub).Error)

		nextGroup := "svip"
		updated, err := AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{UpgradeGroup: &nextGroup})
		require.NoError(t, err)
		assert.Equal(t, nextGroup, updated.UpgradeGroup)
		assert.Equal(t, "default", updated.PrevUserGroup)
		var storedUser User
		require.NoError(t, DB.First(&storedUser, user.Id).Error)
		assert.Equal(t, nextGroup, storedUser.Group)
	})

	t.Run("reactivating subscription applies upgrade and captures fallback", func(t *testing.T) {
		truncateTables(t)
		now := GetDBTimestamp()
		user := &User{Id: 9941, Username: "subscription-group-reactivate", Group: "default", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
		plan := &SubscriptionPlan{Id: 9942, Title: "Group reactivate"}
		require.NoError(t, DB.Create(user).Error)
		require.NoError(t, DB.Create(plan).Error)
		sub := &UserSubscription{
			Id:           9943,
			UserId:       user.Id,
			PlanId:       plan.Id,
			StartTime:    now - 60,
			EndTime:      now + 3600,
			Status:       "expired",
			UpgradeGroup: "pro",
		}
		require.NoError(t, DB.Create(sub).Error)

		status := "active"
		updated, err := AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{Status: &status})
		require.NoError(t, err)
		assert.Equal(t, "default", updated.PrevUserGroup)
		var storedUser User
		require.NoError(t, DB.First(&storedUser, user.Id).Error)
		assert.Equal(t, "pro", storedUser.Group)
	})

	t.Run("clearing active upgrade group reverts current user", func(t *testing.T) {
		truncateTables(t)
		now := GetDBTimestamp()
		user := &User{Id: 9951, Username: "subscription-group-clear", Group: "pro", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
		plan := &SubscriptionPlan{Id: 9952, Title: "Group clear"}
		require.NoError(t, DB.Create(user).Error)
		require.NoError(t, DB.Create(plan).Error)
		sub := &UserSubscription{
			Id:            9953,
			UserId:        user.Id,
			PlanId:        plan.Id,
			StartTime:     now - 60,
			EndTime:       now + 3600,
			Status:        "active",
			UpgradeGroup:  "pro",
			PrevUserGroup: "default",
		}
		require.NoError(t, DB.Create(sub).Error)

		empty := ""
		_, err := AdminUpdateUserSubscription(sub.Id, UserSubscriptionUpdate{UpgradeGroup: &empty})
		require.NoError(t, err)
		var storedUser User
		require.NoError(t, DB.First(&storedUser, user.Id).Error)
		assert.Equal(t, "default", storedUser.Group)
	})
}

func TestSubscriptionActiveWindowRejectsFutureAndUnboundedInstances(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{Id: 9961, Title: "Window plan"}
	user := &User{Id: 9962, Username: "subscription-window", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(user).Error)
	future := &UserSubscription{
		Id:          9963,
		UserId:      user.Id,
		PlanId:      plan.Id,
		StartTime:   now + 3600,
		EndTime:     now + 7200,
		Status:      "active",
		AmountTotal: 1000,
	}
	unbounded := &UserSubscription{
		Id:          9964,
		UserId:      user.Id,
		PlanId:      plan.Id,
		StartTime:   now - 3600,
		EndTime:     0,
		Status:      "active",
		AmountTotal: 1000,
	}
	require.NoError(t, DB.Create(future).Error)
	require.NoError(t, DB.Create(unbounded).Error)

	active, err := HasActiveUserSubscription(user.Id)
	require.NoError(t, err)
	assert.False(t, active)
	_, err = PreConsumeUserSubscription("future-window-request", user.Id, "test-model", 0, 1)
	require.Error(t, err)

	status := "active"
	_, err = AdminUpdateUserSubscription(unbounded.Id, UserSubscriptionUpdate{Status: &status})
	require.Error(t, err)
}

func TestAdminDeleteUserSubscriptionKeepsPendingRefundRecordUsable(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{Id: 9971, Title: "Delete guard"}
	user := &User{Id: 9972, Username: "subscription-delete", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	sub := &UserSubscription{
		Id:          9973,
		UserId:      user.Id,
		PlanId:      plan.Id,
		StartTime:   now - 60,
		EndTime:     now + 3600,
		Status:      "active",
		AmountTotal: 1000,
		AmountUsed:  100,
	}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(sub).Error)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId:          "delete-guard-request",
		UserId:             user.Id,
		UserSubscriptionId: sub.Id,
		PreConsumed:        100,
		Status:             "consumed",
	}).Error)

	_, err := AdminDeleteUserSubscription(sub.Id)
	require.NoError(t, err)
	var stored UserSubscription
	require.NoError(t, DB.First(&stored, sub.Id).Error)
	assert.Equal(t, subscriptionStatusDeleted, stored.Status)
	assert.Zero(t, stored.NextResetTime)
	require.NoError(t, RefundSubscriptionPreConsume("delete-guard-request"))
	assert.Zero(t, getSubscriptionResetSub(t, sub.Id).AmountUsed)
}

func TestAdminDeleteUserSubscriptionKeepsNullPreConsumeStatusRecord(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{Id: 9974, Title: "Delete null guard"}
	user := &User{Id: 9975, Username: "subscription-delete-null", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	sub := &UserSubscription{
		Id:          9976,
		UserId:      user.Id,
		PlanId:      plan.Id,
		StartTime:   now - 60,
		EndTime:     now + 3600,
		Status:      "active",
		AmountTotal: 1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(sub).Error)
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId:          "delete-null-status",
		UserId:             user.Id,
		UserSubscriptionId: sub.Id,
		PreConsumed:        100,
		Status:             "consumed",
	}).Error)
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", "delete-null-status").
		UpdateColumn("status", nil).Error)

	_, err := AdminDeleteUserSubscription(sub.Id)
	require.NoError(t, err)
	var stored UserSubscription
	require.NoError(t, DB.First(&stored, sub.Id).Error)
	assert.Equal(t, subscriptionStatusDeleted, stored.Status)
	var pendingCount int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", "delete-null-status").
		Count(&pendingCount).Error)
	assert.EqualValues(t, 1, pendingCount)
}

func TestSettleSubscriptionPreConsumeMarksSettledAndIsIdempotent(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{Id: 9981, Title: "Settlement status"}
	user := &User{Id: 9982, Username: "subscription-settlement", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	sub := &UserSubscription{
		Id:          9983,
		UserId:      user.Id,
		PlanId:      plan.Id,
		StartTime:   now - 60,
		EndTime:     now + 3600,
		Status:      "active",
		AmountTotal: 1000,
		AmountUsed:  100,
	}
	record := &SubscriptionPreConsumeRecord{
		RequestId:          "settled-request",
		UserId:             user.Id,
		UserSubscriptionId: sub.Id,
		PreConsumed:        100,
		Status:             "consumed",
	}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(sub).Error)
	require.NoError(t, DB.Create(record).Error)

	require.NoError(t, SettleSubscriptionPreConsume(record.RequestId, sub.Id, 25))
	assert.EqualValues(t, 125, getSubscriptionResetSub(t, sub.Id).AmountUsed)
	var storedRecord SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", record.RequestId).First(&storedRecord).Error)
	assert.Equal(t, "settled", storedRecord.Status)

	// A retry must not apply the settlement delta twice.
	require.NoError(t, SettleSubscriptionPreConsume(record.RequestId, sub.Id, 25))
	assert.EqualValues(t, 125, getSubscriptionResetSub(t, sub.Id).AmountUsed)
	require.NoError(t, RefundSubscriptionPreConsume(record.RequestId))
	assert.EqualValues(t, 125, getSubscriptionResetSub(t, sub.Id).AmountUsed)
}

func TestAdminDeleteUserSubscriptionHidesRecordAndKeepsAsyncBillingTarget(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{Id: 9991, Title: "Delete settled"}
	user := &User{Id: 9992, Username: "subscription-delete-settled", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	sub := &UserSubscription{
		Id:          9993,
		UserId:      user.Id,
		PlanId:      plan.Id,
		StartTime:   now - 60,
		EndTime:     now + 3600,
		Status:      "active",
		AmountTotal: 1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(sub).Error)
	for i, status := range []string{"settled", "refunded"} {
		require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
			RequestId:          fmt.Sprintf("delete-safe-%d", i),
			UserId:             user.Id,
			UserSubscriptionId: sub.Id,
			PreConsumed:        100,
			Status:             status,
		}).Error)
	}

	_, err := AdminDeleteUserSubscription(sub.Id)
	require.NoError(t, err)
	var stored UserSubscription
	require.NoError(t, DB.First(&stored, sub.Id).Error)
	assert.Equal(t, subscriptionStatusDeleted, stored.Status)
	require.NoError(t, PostConsumeUserSubscriptionDelta(sub.Id, 25))
	assert.EqualValues(t, 25, getSubscriptionResetSub(t, sub.Id).AmountUsed)

	summaries, err := GetAllUserSubscriptions(user.Id)
	require.NoError(t, err)
	assert.Empty(t, summaries)
	listRecords, total, err := ListAdminSubscriptions(
		&common.PageInfo{Page: 1, PageSize: 20},
		AdminSubscriptionQueryOptions{},
	)
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, listRecords)
	count, err := CountUserSubscriptionsByPlan(user.Id, plan.Id)
	require.NoError(t, err)
	assert.Zero(t, count)

	var records []SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("user_subscription_id = ?", sub.Id).Find(&records).Error)
	assert.Len(t, records, 2)
}

func TestCleanupSubscriptionPreConsumeRecordsKeepsPendingRecords(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	for i, status := range []string{"consumed", "settled", "refunded"} {
		require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
			RequestId:          fmt.Sprintf("cleanup-%d", i),
			UserId:             10000,
			UserSubscriptionId: 10001,
			PreConsumed:        100,
			Status:             status,
		}).Error)
	}
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).
		Where("request_id IN ?", []string{"cleanup-0", "cleanup-1", "cleanup-2"}).
		UpdateColumn("updated_at", now-3600).Error)

	deleted, err := CleanupSubscriptionPreConsumeRecords(60)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)
	var pending SubscriptionPreConsumeRecord
	require.NoError(t, DB.Where("request_id = ?", "cleanup-0").First(&pending).Error)
	assert.Equal(t, "consumed", pending.Status)
}
