package service

import (
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFinancialBaseQuotaExcludesGroupRatio(t *testing.T) {
	priceData := types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.25}}
	baseQuota, covered := financialBaseQuota(2500, priceData, nil)

	assert.True(t, covered)
	assert.Equal(t, 2000.0, baseQuota)
}

func TestFinancialBaseQuotaUsesTieredExactValue(t *testing.T) {
	priceData := types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 9}}
	exact := 1234.5
	baseQuota, covered := financialBaseQuota(9999, priceData, &exact)

	assert.True(t, covered)
	assert.Equal(t, exact, baseQuota)
}

func TestFinancialBaseQuotaUsesModelPriceForFreeGroup(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	priceData := types.PriceData{
		UsePrice:       true,
		ModelPrice:     0.02,
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0},
	}
	priceData.AddOtherRatio("duration", 1.5)
	baseQuota, covered := financialBaseQuota(0, priceData, nil)

	assert.True(t, covered)
	assert.Equal(t, 15000.0, baseQuota)
}

func TestFinancialBaseQuotaMarksFreeRatioModeUncovered(t *testing.T) {
	baseQuota, covered := financialBaseQuota(0, types.PriceData{}, nil)

	assert.False(t, covered)
	assert.Zero(t, baseQuota)
}

func TestFinancialBaseQuotaRejectsOverflowingGroupRatioDivision(t *testing.T) {
	priceData := types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: math.SmallestNonzeroFloat64}}

	baseQuota, covered := financialBaseQuota(1, priceData, nil)

	assert.False(t, covered)
	assert.Zero(t, baseQuota)
}

func TestTextFinancialBaseQuotaCoversFreeGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		StartTime:       time.Now(),
		PriceData: types.PriceData{
			ModelRatio:      2,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 0},
		},
	}
	usage := &dto.Usage{PromptTokens: 100, TotalTokens: 100}
	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	baseQuota := textFinancialBaseQuota(ctx, relayInfo, usage, summary, nil)

	require.NotNil(t, baseQuota)
	assert.Equal(t, 200.0, *baseQuota)
}

func TestTextFinancialBaseQuotaUsesTieredResult(t *testing.T) {
	summary := textQuotaSummary{GroupRatio: 2}
	result := &billingexpr.TieredResult{ActualQuotaBeforeGroup: 1234}

	baseQuota := textFinancialBaseQuota(nil, nil, nil, summary, result)

	require.NotNil(t, baseQuota)
	assert.Equal(t, 1234.0, *baseQuota)
}

func TestTextFinancialBaseQuotaExcludesPaidGroupRate(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "fixed-model",
		PriceData: types.PriceData{
			UsePrice:       true,
			ModelPrice:     0.02,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
		},
	}
	usage := &dto.Usage{PromptTokens: 1, TotalTokens: 1}
	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	baseQuota := textFinancialBaseQuota(ctx, relayInfo, usage, summary, nil)
	require.NotNil(t, baseQuota)
	assert.Equal(t, 10000.0, *baseQuota)
}

func TestAudioFinancialBaseQuotaExcludesGroupRateForFixedPrice(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	baseQuota := audioFinancialBaseQuota(QuotaInfo{
		UsePrice:   true,
		GroupRatio: 2,
	}, 0.02, 1)

	require.NotNil(t, baseQuota)
	assert.Equal(t, 10000.0, *baseQuota)
}

func TestRecordChannelFinancialConsumeDoesNotAffectUserBilling(t *testing.T) {
	previousDB := model.DB
	previousQuotaPerUnit := common.QuotaPerUnit
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ChannelFinancialRecord{}))
	model.DB = db
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	user := &model.User{Username: "financial-user", Quota: 1000}
	require.NoError(t, model.DB.Create(user).Error)
	relayInfo := &relaycommon.RelayInfo{
		UserId: user.Id,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:       7,
			ChannelName:     "channel",
			ChannelCostRate: 0.25,
		},
		PriceData: types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}},
	}

	RecordChannelFinancialConsume(nil, relayInfo, "gpt-test", 500, nil)

	var storedUser model.User
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	assert.Equal(t, 1000, storedUser.Quota)
	var record model.ChannelFinancialRecord
	require.NoError(t, model.DB.First(&record).Error)
	assert.Equal(t, 0.001, record.RevenueUSD)
	assert.Equal(t, 0.0005, record.BaseCostUSD)
	assert.Equal(t, 0.000125, record.ChannelCostUSD)
}

func TestRecordChannelFinancialConsumeKeepsSelectedSnapshotAfterRateChange(t *testing.T) {
	previousDB := model.DB
	previousQuotaPerUnit := common.QuotaPerUnit
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelCostRateVersion{}, &model.ChannelFinancialRecord{}))
	model.DB = db
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	channel := &model.Channel{Key: "key", Name: "channel", CostRate: 0.8}
	require.NoError(t, model.DB.Create(channel).Error)
	initialVersion := &model.ChannelCostRateVersion{ChannelId: channel.Id, CostRate: 0.8, EffectiveAt: 0}
	require.NoError(t, model.DB.Create(initialVersion).Error)
	selectedSnapshot, err := model.GetCurrentChannelCostRateSnapshot(channel.Id, channel.CostRate)
	require.NoError(t, err)
	relayInfo := &relaycommon.RelayInfo{
		RequestId: "before-rate-change",
		StartTime: time.Unix(150, 0),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:                  channel.Id,
			ChannelName:                channel.Name,
			ChannelCostRate:            selectedSnapshot.CostRate,
			ChannelCostRateVersionId:   selectedSnapshot.VersionId,
			ChannelCostRateEffectiveAt: selectedSnapshot.EffectiveAt,
		},
		PriceData: types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	require.NoError(t, model.DB.Model(channel).Update("cost_rate", 1.2).Error)
	require.NoError(t, model.DB.Create(&model.ChannelCostRateVersion{
		ChannelId:   channel.Id,
		CostRate:    1.2,
		EffectiveAt: time.Unix(200, 0).UnixNano(),
	}).Error)

	RecordChannelFinancialConsume(nil, relayInfo, "gpt-test", 500000, nil)

	var record model.ChannelFinancialRecord
	require.NoError(t, model.DB.First(&record).Error)
	assert.Equal(t, 0.8, record.CostRate)
	assert.Equal(t, 0.8, record.ChannelCostUSD)
	assert.Equal(t, initialVersion.Id, record.CostRateVersionId)
	assert.Equal(t, int64(150), record.CreatedAt)
}

func TestTaskFinancialAdjustmentKeepsSubmittedRateVersion(t *testing.T) {
	previousDB := model.DB
	previousQuotaPerUnit := common.QuotaPerUnit
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelCostRateVersion{}, &model.ChannelFinancialRecord{}))
	model.DB = db
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	channel := &model.Channel{Key: "key", Name: "channel", CostRate: 0.8}
	require.NoError(t, model.DB.Create(channel).Error)
	initialVersion := &model.ChannelCostRateVersion{ChannelId: channel.Id, CostRate: 0.8, EffectiveAt: 0}
	require.NoError(t, model.DB.Create(initialVersion).Error)
	task := &model.Task{
		TaskID:     "task-version-snapshot",
		ChannelId:  channel.Id,
		Properties: model.Properties{OriginModelName: "gpt-test"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			GroupRatio:                 1,
			ChannelName:                channel.Name,
			ChannelCostRate:            initialVersion.CostRate,
			ChannelCostRateSet:         true,
			ChannelCostRateVersionId:   initialVersion.Id,
			ChannelCostRateEffectiveAt: initialVersion.EffectiveAt,
			QuotaPerUnit:               common.QuotaPerUnit,
		}},
	}
	require.NoError(t, model.DB.Model(channel).Update("cost_rate", 1.2).Error)
	require.NoError(t, model.DB.Create(&model.ChannelCostRateVersion{
		ChannelId:   channel.Id,
		CostRate:    1.2,
		EffectiveAt: time.Unix(200, 0).UnixNano(),
	}).Error)

	RecordChannelFinancialTaskAdjustment(task, model.FinancialEventConsume, 500000)

	var record model.ChannelFinancialRecord
	require.NoError(t, model.DB.First(&record).Error)
	assert.Equal(t, 0.8, record.CostRate)
	assert.Equal(t, initialVersion.Id, record.CostRateVersionId)
	assert.Equal(t, 0.8, record.ChannelCostUSD)
}

func TestFinancialRefundKeepsTheOriginalRateSnapshot(t *testing.T) {
	previousDB := model.DB
	previousQuotaPerUnit := common.QuotaPerUnit
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelFinancialRecord{}))
	model.DB = db
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	channel := &model.Channel{Key: "key", Name: "channel", CostRate: 0.25}
	require.NoError(t, model.DB.Create(channel).Error)
	relayInfo := &relaycommon.RelayInfo{
		RequestId: "request-1",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:       channel.Id,
			ChannelName:     channel.Name,
			ChannelCostRate: channel.CostRate,
		},
		PriceData: types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	RecordChannelFinancialConsume(nil, relayInfo, "gpt-test", 500000, nil)
	require.NoError(t, model.DB.Model(channel).Update("cost_rate", 0.9).Error)
	RecordChannelFinancialRefund("request-1", channel.Id, "gpt-test", 500000)

	var records []model.ChannelFinancialRecord
	require.NoError(t, model.DB.Order("id ASC").Find(&records).Error)
	require.Len(t, records, 2)
	assert.Equal(t, 0.25, records[1].CostRate)
	assert.Equal(t, -1.0, records[1].RevenueUSD)
	assert.Zero(t, records[1].ChannelCostUSD)
}

func TestFinancialRefundUsesOriginalQuotaPerUnit(t *testing.T) {
	previousDB := model.DB
	previousQuotaPerUnit := common.QuotaPerUnit
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelFinancialRecord{}))
	model.DB = db
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	channel := &model.Channel{Key: "key", Name: "channel", CostRate: 0.5}
	require.NoError(t, model.DB.Create(channel).Error)
	relayInfo := &relaycommon.RelayInfo{
		RequestId: "request-qpu",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:       channel.Id,
			ChannelName:     channel.Name,
			ChannelCostRate: channel.CostRate,
		},
		PriceData: types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	RecordChannelFinancialConsume(nil, relayInfo, "gpt-test", 500000, nil)

	common.QuotaPerUnit = 1000000
	RecordChannelFinancialRefund("request-qpu", channel.Id, "gpt-test", 500000)

	var records []model.ChannelFinancialRecord
	require.NoError(t, model.DB.Order("id ASC").Find(&records).Error)
	require.Len(t, records, 2)
	assert.Equal(t, 500000.0, records[1].QuotaPerUnit)
	assert.Equal(t, -1.0, records[1].RevenueUSD)
}

func TestFinancialRefundResolvesAsynchronousReferenceSnapshot(t *testing.T) {
	previousDB := model.DB
	previousQuotaPerUnit := common.QuotaPerUnit
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelFinancialRecord{}))
	model.DB = db
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		model.DB = previousDB
		common.QuotaPerUnit = previousQuotaPerUnit
	})

	channel := &model.Channel{Key: "key", Name: "channel", CostRate: 0.25}
	require.NoError(t, model.DB.Create(channel).Error)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set(common.RequestIdKey, "http-request")
	relayInfo := &relaycommon.RelayInfo{
		RequestId: "http-request",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:       channel.Id,
			ChannelName:     channel.Name,
			ChannelCostRate: channel.CostRate,
		},
		PriceData: types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	RecordChannelFinancialConsumeWithReference(ctx, relayInfo, "gpt-test", 500000, nil, "task-id")
	require.NoError(t, model.DB.Model(channel).Update("cost_rate", 0.9).Error)

	RecordChannelFinancialRefund("task-id", channel.Id, "gpt-test", 500000)

	var records []model.ChannelFinancialRecord
	require.NoError(t, model.DB.Order("id ASC").Find(&records).Error)
	require.Len(t, records, 2)
	assert.Equal(t, "http-request", records[0].RequestId)
	assert.Equal(t, "task-id", records[0].ReferenceId)
	assert.Equal(t, 0.25, records[1].CostRate)
}
