package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupFinancialControllerTest(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousQuotaPerUnit := common.QuotaPerUnit
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Log{}, &model.ChannelFinancialRecord{}))
	model.DB, model.LOG_DB = db, db
	common.QuotaPerUnit = 50
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.QuotaPerUnit = previousQuotaPerUnit
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
}

func TestGetFinancialReportCombinesExactAndHistoricalRecords(t *testing.T) {
	setupFinancialControllerTest(t)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 7, Key: "key", Name: "primary", CostRate: 0.5}).Error)
	require.NoError(t, model.DB.Create(&model.ChannelFinancialRecord{
		CreatedAt:      100,
		EventType:      model.FinancialEventConsume,
		RequestId:      "exact",
		ChannelId:      7,
		ChannelName:    "primary",
		ModelName:      "gpt-test",
		CostRate:       0.5,
		RevenueUSD:     2,
		BaseCostUSD:    2,
		ChannelCostUSD: 1,
		Quota:          100,
		QuotaPerUnit:   50,
		Covered:        true,
	}).Error)
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{CreatedAt: 101, Type: model.LogTypeConsume, RequestId: "historical", ChannelId: 7, ModelName: "gpt-test", Quota: 100, Other: `{"group_ratio":2}`},
		{CreatedAt: 102, Type: model.LogTypeRefund, RequestId: "refund", ChannelId: 7, ModelName: "gpt-test", Quota: 50},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/data/financial?start_timestamp=99&end_timestamp=103&channel_id=7&model_name=gpt-test", nil)

	GetFinancialReport(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                         `json:"success"`
		Data    model.ChannelFinancialReport `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, 3.0, response.Data.Summary.RevenueUSD)
	assert.Equal(t, 1.5, response.Data.Summary.CostUSD)
	assert.Equal(t, 1.5, response.Data.Summary.ProfitUSD)
	assert.Equal(t, int64(2), response.Data.Summary.RequestCount)
	assert.Equal(t, int64(2), response.Data.Summary.EstimatedCount)
	require.Len(t, response.Data.ByChannel, 1)
	assert.Equal(t, "primary", response.Data.ByChannel[0].Name)
}

func TestGetFinancialReportRejectsInvalidRangeAndChannel(t *testing.T) {
	setupFinancialControllerTest(t)

	tests := []string{
		"/api/data/financial?start_timestamp=1&end_timestamp=7776002",
		"/api/data/financial?start_timestamp=1&end_timestamp=2&channel_id=-1",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, target, nil)

			GetFinancialReport(c)

			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
		})
	}
}

func TestParseFinancialReportRangeDefaultsToThirtyDays(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/data/financial", nil)

	before := time.Now().Unix()
	start, end, ok := parseFinancialReportRange(c)
	after := time.Now().Unix()

	require.True(t, ok)
	assert.GreaterOrEqual(t, end, before)
	assert.LessOrEqual(t, end, after)
	assert.InDelta(t, 30*24*time.Hour.Seconds(), float64(end-start), 1)
}

func TestParseFinancialReportRangeAllowsExactlyNinetyDays(t *testing.T) {
	const start = int64(1_700_000_000)
	end := start + int64(maxFinancialReportRange/time.Second)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/data/financial?start_timestamp="+strconv.FormatInt(start, 10)+"&end_timestamp="+strconv.FormatInt(end, 10), nil)

	actualStart, actualEnd, ok := parseFinancialReportRange(c)

	require.True(t, ok)
	assert.Equal(t, start, actualStart)
	assert.Equal(t, end, actualEnd)
}
