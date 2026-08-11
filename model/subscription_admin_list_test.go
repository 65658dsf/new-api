package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListAdminSubscriptionsReturnsSubscriberPlanAndQuota(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	alice := &User{
		Id:          9701,
		Username:    "subscription-alice",
		DisplayName: "Alice",
		Email:       "subscription-alice@example.com",
		AffCode:     "subscription-alice-code",
	}
	bob := &User{
		Id:          9702,
		Username:    "subscription-bob",
		DisplayName: "Bob",
		Email:       "subscription-bob@example.com",
		AffCode:     "subscription-bob-code",
	}
	require.NoError(t, DB.Create(alice).Error)
	require.NoError(t, DB.Create(bob).Error)

	pro := &SubscriptionPlan{Id: 9801, Title: "Admin List Pro"}
	basic := &SubscriptionPlan{Id: 9802, Title: "Admin List Basic"}
	require.NoError(t, DB.Create(pro).Error)
	require.NoError(t, DB.Create(basic).Error)

	require.NoError(t, DB.Create(&UserSubscription{
		Id:          9901,
		UserId:      alice.Id,
		PlanId:      pro.Id,
		AmountTotal: 1000,
		AmountUsed:  275,
		StartTime:   now - 60,
		EndTime:     now + 3600,
		Status:      "active",
		Source:      "order",
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:          9902,
		UserId:      bob.Id,
		PlanId:      basic.Id,
		AmountTotal: 500,
		AmountUsed:  500,
		StartTime:   now - 7200,
		EndTime:     now - 60,
		Status:      "expired",
		Source:      "admin",
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:          9903,
		UserId:      alice.Id,
		PlanId:      basic.Id,
		AmountTotal: 0,
		AmountUsed:  0,
		StartTime:   now - 120,
		EndTime:     now + 7200,
		Status:      "cancelled",
		Source:      "admin",
	}).Error)

	items, total, err := ListAdminSubscriptions(&common.PageInfo{Page: 1, PageSize: 10}, AdminSubscriptionQueryOptions{
		Keyword: "subscription-alice",
		Status:  "active",
	})
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)

	item := items[0]
	assert.Equal(t, 9901, item.Subscription.Id)
	assert.Equal(t, int64(1000), item.Subscription.AmountTotal)
	assert.Equal(t, int64(275), item.Subscription.AmountUsed)
	assert.Equal(t, "active", item.Subscription.Status)
	require.NotNil(t, item.User)
	assert.Equal(t, alice.Id, item.User.Id)
	assert.Equal(t, alice.Username, item.User.Username)
	assert.Equal(t, alice.Email, item.User.Email)
	require.NotNil(t, item.Plan)
	assert.Equal(t, pro.Id, item.Plan.Id)
	assert.Equal(t, pro.Title, item.Plan.Title)

	items, total, err = ListAdminSubscriptions(&common.PageInfo{Page: 1, PageSize: 2}, AdminSubscriptionQueryOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, items, 2)
	assert.Equal(t, 9903, items[0].Subscription.Id)
	assert.Equal(t, 9902, items[1].Subscription.Id)
}

func TestListAdminSubscriptionsFiltersByEffectiveExpiration(t *testing.T) {
	truncateTables(t)

	now := GetDBTimestamp()
	plan := &SubscriptionPlan{Id: 9811, Title: "Effective Status"}
	otherPlan := &SubscriptionPlan{Id: 9812, Title: "Other Status"}
	require.NoError(t, DB.Create(plan).Error)
	require.NoError(t, DB.Create(otherPlan).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:        9911,
		UserId:    9711,
		PlanId:    plan.Id,
		StartTime: now - 7200,
		EndTime:   now - 60,
		Status:    "active",
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:        9912,
		UserId:    9712,
		PlanId:    otherPlan.Id,
		StartTime: now - 7200,
		EndTime:   now - 60,
		Status:    "expired",
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		Id:        9913,
		UserId:    9713,
		PlanId:    plan.Id,
		StartTime: now - 7200,
		EndTime:   0,
		Status:    "active",
	}).Error)

	activeItems, activeTotal, err := ListAdminSubscriptions(
		&common.PageInfo{Page: 1, PageSize: 10},
		AdminSubscriptionQueryOptions{Status: "active"},
	)
	require.NoError(t, err)
	assert.Zero(t, activeTotal)
	assert.Empty(t, activeItems)

	expiredItems, expiredTotal, err := ListAdminSubscriptions(
		&common.PageInfo{Page: 1, PageSize: 10},
		AdminSubscriptionQueryOptions{Status: "expired"},
	)
	require.NoError(t, err)
	assert.EqualValues(t, 3, expiredTotal)
	require.Len(t, expiredItems, 3)
	assert.Equal(t, 9913, expiredItems[0].Subscription.Id)
	assert.Equal(t, 9912, expiredItems[1].Subscription.Id)
	assert.Equal(t, 9911, expiredItems[2].Subscription.Id)

	filteredItems, filteredTotal, err := ListAdminSubscriptions(
		&common.PageInfo{Page: 1, PageSize: 10},
		AdminSubscriptionQueryOptions{Status: "expired", PlanId: plan.Id},
	)
	require.NoError(t, err)
	assert.EqualValues(t, 2, filteredTotal)
	require.Len(t, filteredItems, 2)
	assert.Equal(t, 9913, filteredItems[0].Subscription.Id)
	assert.Equal(t, 9911, filteredItems[1].Subscription.Id)

	_, _, err = ListAdminSubscriptions(
		&common.PageInfo{Page: 1, PageSize: -1},
		AdminSubscriptionQueryOptions{},
	)
	require.Error(t, err)
}
