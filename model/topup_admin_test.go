package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTopUpAdminUser(t *testing.T, user User) {
	t.Helper()
	require.NoError(t, DB.Create(&user).Error)
}

func seedTopUpAdminOrder(t *testing.T, order TopUp) {
	t.Helper()
	require.NoError(t, DB.Create(&order).Error)
}

func TestGetAllTopUpRecordsFiltersAndAttachesUsers(t *testing.T) {
	truncateTables(t)

	seedTopUpAdminUser(t, User{
		Id:          701,
		Username:    "alice",
		DisplayName: "Alice",
		Email:       "alice@example.com",
		AffCode:     "ta701",
		Status:      common.UserStatusEnabled,
	})
	seedTopUpAdminUser(t, User{
		Id:          702,
		Username:    "bob",
		DisplayName: "Bob",
		Email:       "bob@example.com",
		AffCode:     "ta702",
		Status:      common.UserStatusEnabled,
	})

	now := time.Now().Unix()
	seedTopUpAdminOrder(t, TopUp{
		Id:              801,
		UserId:          701,
		Amount:          10,
		Money:           10,
		TradeNo:         "order-alice-success",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      now - 10,
		CompleteTime:    now - 5,
		Status:          common.TopUpStatusSuccess,
	})
	seedTopUpAdminOrder(t, TopUp{
		Id:              802,
		UserId:          702,
		Amount:          20,
		Money:           20,
		TradeNo:         "order-bob-pending",
		PaymentMethod:   "wxpay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      now - 20,
		Status:          common.TopUpStatusPending,
	})

	page := &common.PageInfo{Page: 1, PageSize: 10}
	records, total, err := GetAllTopUpRecords(page, TopUpQueryOptions{
		Status:        common.TopUpStatusSuccess,
		PaymentMethod: "alipay",
	})

	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "order-alice-success", records[0].TradeNo)
	require.NotNil(t, records[0].User)
	assert.Equal(t, 701, records[0].User.Id)
	assert.Equal(t, "alice@example.com", records[0].User.Email)

	records, total, err = GetAllTopUpRecords(page, TopUpQueryOptions{
		Keyword: "alice",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, 701, records[0].UserId)

	records, total, err = GetAllTopUpRecords(page, TopUpQueryOptions{
		Keyword: "bob@example.com",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "order-bob-pending", records[0].TradeNo)

	records, total, err = GetAllTopUpRecords(page, TopUpQueryOptions{
		Keyword: "alice-success",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "order-alice-success", records[0].TradeNo)

	records, total, err = GetAllTopUpRecords(&common.PageInfo{Page: 2, PageSize: 1}, TopUpQueryOptions{})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, records, 1)
	assert.Equal(t, "order-alice-success", records[0].TradeNo)
}

func TestGetTopUpOverviewAggregatesSuccessfulOrders(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.Local)
	today := time.Date(2026, 6, 22, 9, 0, 0, 0, time.Local).Unix()
	yesterday := time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local).Unix()
	outOfRange := time.Date(2026, 6, 1, 10, 0, 0, 0, time.Local).Unix()

	seedTopUpAdminUser(t, User{Id: 901, Username: "top_user", Email: "top@example.com", AffCode: "ta901", Status: common.UserStatusEnabled})
	seedTopUpAdminUser(t, User{Id: 902, Username: "small_user", Email: "small@example.com", AffCode: "ta902", Status: common.UserStatusEnabled})

	seedTopUpAdminOrder(t, TopUp{
		UserId:          901,
		Amount:          30,
		Money:           30,
		TradeNo:         "today-success",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      today - 60,
		CompleteTime:    today,
		Status:          common.TopUpStatusSuccess,
	})
	seedTopUpAdminOrder(t, TopUp{
		UserId:          901,
		Amount:          20,
		Money:           20,
		TradeNo:         "yesterday-success",
		PaymentMethod:   "wxpay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      yesterday - 60,
		CompleteTime:    yesterday,
		Status:          common.TopUpStatusSuccess,
	})
	seedTopUpAdminOrder(t, TopUp{
		UserId:          902,
		Amount:          99,
		Money:           99,
		TradeNo:         "pending-ignored",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      today,
		Status:          common.TopUpStatusPending,
	})
	seedTopUpAdminOrder(t, TopUp{
		UserId:          902,
		Amount:          100,
		Money:           100,
		TradeNo:         "old-success",
		PaymentMethod:   "stripe",
		PaymentProvider: PaymentProviderStripe,
		CreateTime:      outOfRange,
		CompleteTime:    outOfRange,
		Status:          common.TopUpStatusSuccess,
	})

	overview, err := GetTopUpOverview(7, now)

	require.NoError(t, err)
	assert.Equal(t, 30.0, overview.TodayIncome)
	assert.Equal(t, 1, overview.TodayOrders)
	assert.Equal(t, 50.0, overview.RangeIncome)
	assert.Equal(t, 2, overview.RangeOrders)
	assert.Equal(t, 25.0, overview.AverageAmount)
	require.Len(t, overview.Daily, 7)
	assert.Equal(t, "2026-06-16", overview.Daily[0].Date)
	assert.Equal(t, "2026-06-22", overview.Daily[6].Date)
	assert.Equal(t, 30.0, overview.Daily[6].Income)
	require.Len(t, overview.PaymentMethods, 2)
	assert.Equal(t, "alipay", overview.PaymentMethods[0].PaymentMethod)
	require.Len(t, overview.TopUsers, 1)
	assert.Equal(t, 901, overview.TopUsers[0].User.Id)
	assert.Equal(t, 50.0, overview.TopUsers[0].Income)
}
