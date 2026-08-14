package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestBuildChannelFinancialReport(t *testing.T) {
	rows := []*ChannelFinancialRecord{
		{CreatedAt: 1722470400, EventType: FinancialEventConsume, ChannelId: 2, ChannelName: "west", ModelName: "gpt-b", RevenueUSD: 4, ChannelCostUSD: 1, Covered: true},
		{CreatedAt: 1722384000, EventType: FinancialEventConsume, ChannelId: 1, ChannelName: "east", ModelName: "gpt-a", RevenueUSD: 10, ChannelCostUSD: 6, Covered: true},
		{CreatedAt: 1722470400, EventType: FinancialEventRefund, ChannelId: 1, ChannelName: "east", ModelName: "gpt-a", RevenueUSD: -2, Covered: true},
		{CreatedAt: 1722470400, EventType: FinancialEventConsume, ChannelId: 3, ChannelName: "unknown", ModelName: "gpt-c", RevenueUSD: 3, Estimated: true},
	}

	report := BuildChannelFinancialReport(rows, 1722384000, 1722470400)

	assert.Equal(t, 15.0, report.Summary.RevenueUSD)
	assert.Equal(t, 7.0, report.Summary.CostUSD)
	assert.Equal(t, 8.0, report.Summary.ProfitUSD)
	assert.InDelta(t, 8.0/15.0, report.Summary.ProfitMargin, 1e-12)
	assert.Equal(t, int64(3), report.Summary.RequestCount)
	assert.Equal(t, int64(3), report.Summary.CoveredCount)
	assert.Equal(t, int64(3), report.Summary.ExactCount)
	assert.Equal(t, int64(1), report.Summary.UncoveredCount)
	assert.Equal(t, int64(1), report.Summary.EstimatedCount)
	require.Len(t, report.Trend, 2)
	assert.Equal(t, "2024-07-31", report.Trend[0].Date)
	assert.Equal(t, int64(2), report.Trend[1].RequestCount)
	assert.Equal(t, "east", report.ByChannel[0].Name)
	assert.Equal(t, "gpt-a", report.ByModel[0].ModelName)
}

func TestChannelFinancialLaunchOptionConditionIsMySQLSafe(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "root:root@tcp(127.0.0.1:3306)/test",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)

	result := db.Where(&Option{Key: ChannelFinancialLaunchOptionKey}).First(&Option{})
	require.NoError(t, result.Error)
	assert.Contains(t, result.Statement.SQL.String(), "`key` = ?")
}

func TestGetHistoricalChannelFinancialRecords(t *testing.T) {
	setupChannelCostRateTestDB(t)
	previousLogDB := LOG_DB
	previousQuotaPerUnit := common.QuotaPerUnit
	LOG_DB = DB
	common.QuotaPerUnit = 500000
	require.NoError(t, DB.AutoMigrate(&Log{}, &ChannelFinancialRecord{}))
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})
	require.NoError(t, DB.Create(&Channel{Id: 7, Key: "key", Name: "channel", CostRate: 0.5}).Error)
	require.NoError(t, DB.Create(&Log{CreatedAt: 100, Type: LogTypeConsume, RequestId: "historical", ChannelId: 7, ModelName: "gpt", Quota: 1000, Other: `{"group_ratio":2}`}).Error)
	require.NoError(t, DB.Create(&Log{CreatedAt: 101, Type: LogTypeConsume, RequestId: "exact", ChannelId: 7, ModelName: "gpt", Quota: 2000, Other: `{"group_ratio":2}`}).Error)
	require.NoError(t, DB.Create(&Log{CreatedAt: 102, Type: LogTypeConsume, RequestId: "uncovered", ChannelId: 7, ModelName: "gpt", Quota: 3000, Other: `{"group_ratio":0}`}).Error)
	require.NoError(t, DB.Create(&Log{CreatedAt: 103, Type: LogTypeConsume, RequestId: "channel-test", ChannelId: 7, ModelName: "gpt", TokenName: "模型测试", Quota: 4000, Other: `{"group_ratio":2}`}).Error)

	rows, err := GetHistoricalChannelFinancialRecords(99, 104, 0, "", []*ChannelFinancialRecord{{RequestId: "exact", EventType: FinancialEventConsume, ChannelId: 7, ModelName: "gpt", Quota: 2000}})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "historical", rows[0].RequestId)
	assert.True(t, rows[0].Covered)
	assert.True(t, rows[0].Estimated)
	assert.Equal(t, 0.002, rows[0].RevenueUSD)
	assert.Equal(t, 0.001, rows[0].BaseCostUSD)
	assert.Equal(t, 0.0005, rows[0].ChannelCostUSD)
	assert.Equal(t, "uncovered", rows[1].RequestId)
	assert.False(t, rows[1].Covered)
}

func TestHistoricalFinancialRecordsSkipExactRowsWithoutRequestID(t *testing.T) {
	setupChannelCostRateTestDB(t)
	previousLogDB := LOG_DB
	previousQuotaPerUnit := common.QuotaPerUnit
	LOG_DB = DB
	common.QuotaPerUnit = 500000
	require.NoError(t, DB.AutoMigrate(&Log{}, &ChannelFinancialRecord{}))
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})
	require.NoError(t, DB.Create(&Log{
		CreatedAt: 100,
		Type:      LogTypeConsume,
		ChannelId: 7,
		ModelName: "gpt",
		Quota:     1000,
		Other:     `{"group_ratio":2}`,
	}).Error)

	rows, err := GetHistoricalChannelFinancialRecords(99, 101, 0, "", []*ChannelFinancialRecord{{
		CreatedAt: 100,
		EventType: FinancialEventConsume,
		ChannelId: 7,
		ModelName: "gpt",
		Quota:     1000,
	}})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestHistoricalFinancialRecordsRespectFinancialLaunchBoundary(t *testing.T) {
	setupChannelCostRateTestDB(t)
	previousLogDB := LOG_DB
	previousQuotaPerUnit := common.QuotaPerUnit
	LOG_DB = DB
	common.QuotaPerUnit = 500000
	require.NoError(t, DB.AutoMigrate(&Log{}, &Option{}, &ChannelFinancialRecord{}))
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	require.NoError(t, DB.Create(&Channel{Id: 7, Key: "key", Name: "channel", CostRate: 0.5}).Error)
	require.NoError(t, DB.Create(&Option{
		Key:   ChannelFinancialLaunchOptionKey,
		Value: "101",
	}).Error)
	require.NoError(t, DB.Create(&[]Log{
		{CreatedAt: 100, Type: LogTypeConsume, RequestId: "before-launch", ChannelId: 7, ModelName: "gpt", Quota: 1000, Other: `{"group_ratio":2}`},
		{CreatedAt: 101, Type: LogTypeConsume, RequestId: "at-launch", ChannelId: 7, ModelName: "gpt", Quota: 1000, Other: `{"group_ratio":2}`},
	}).Error)

	rows, err := GetHistoricalChannelFinancialRecords(99, 102, 0, "", nil)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "before-launch", rows[0].RequestId)
	assert.Equal(t, "at-launch", rows[1].RequestId)
	assert.False(t, rows[1].Covered)
}

func TestHistoricalFinancialRecordsDoNotTurnNegativeConsumeQuotaIntoRevenue(t *testing.T) {
	setupChannelCostRateTestDB(t)
	previousLogDB := LOG_DB
	previousQuotaPerUnit := common.QuotaPerUnit
	LOG_DB = DB
	common.QuotaPerUnit = 500000
	require.NoError(t, DB.AutoMigrate(&Log{}, &Option{}, &ChannelFinancialRecord{}))
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	require.NoError(t, DB.Create(&Channel{Id: 7, Key: "key", Name: "channel", CostRate: 0.5}).Error)
	require.NoError(t, DB.Create(&Log{
		CreatedAt: 100,
		Type:      LogTypeConsume,
		RequestId: "negative",
		ChannelId: 7,
		ModelName: "gpt",
		Quota:     -1000,
		Other:     `{"group_ratio":2}`,
	}).Error)

	rows, err := GetHistoricalChannelFinancialRecords(99, 101, 0, "", nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Zero(t, rows[0].RevenueUSD)
	assert.False(t, rows[0].Covered)
}

func TestHistoricalFinancialRecordsExposePostLaunchUncoveredLogs(t *testing.T) {
	setupChannelCostRateTestDB(t)
	previousLogDB := LOG_DB
	previousQuotaPerUnit := common.QuotaPerUnit
	LOG_DB = DB
	common.QuotaPerUnit = 500000
	require.NoError(t, DB.AutoMigrate(&Log{}, &Option{}, &ChannelFinancialRecord{}))
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	require.NoError(t, DB.Create(&Channel{Id: 7, Key: "key", Name: "channel", CostRate: 0.5}).Error)
	require.NoError(t, DB.Create(&Option{
		Key:   ChannelFinancialLaunchOptionKey,
		Value: "100",
	}).Error)
	require.NoError(t, DB.Create(&[]Log{
		{CreatedAt: 100, Type: LogTypeConsume, RequestId: "settled", ChannelId: 7, ModelName: "gpt", Quota: 1000, Other: `{"financial_settled":true}`},
		{CreatedAt: 101, Type: LogTypeConsume, RequestId: "failed", ChannelId: 7, ModelName: "gpt", Quota: 1000, Other: `{"financial_settled":false}`},
		{CreatedAt: 102, Type: LogTypeConsume, RequestId: "unknown", ChannelId: 7, ModelName: "gpt", Quota: 1000, Other: `{}`},
	}).Error)

	rows, err := GetHistoricalChannelFinancialRecords(100, 102, 0, "", nil)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "settled", rows[0].RequestId)
	assert.Equal(t, 0.002, rows[0].RevenueUSD)
	assert.False(t, rows[0].Covered)
	assert.Equal(t, "failed", rows[1].RequestId)
	assert.Zero(t, rows[1].RevenueUSD)
	assert.False(t, rows[1].Covered)
	assert.False(t, rows[1].Estimated)
	assert.Equal(t, "unknown", rows[2].RequestId)
	assert.Zero(t, rows[2].RevenueUSD)
	assert.False(t, rows[2].Covered)
	assert.False(t, rows[2].Estimated)
}
