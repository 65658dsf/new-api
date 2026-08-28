package service

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func isFiniteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func markFinancialSettlement(other map[string]interface{}, succeeded bool) {
	if other != nil {
		other["financial_settled"] = succeeded
	}
}

func RecordChannelFinancialConsume(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string, quota int, baseQuotaBeforeGroup *float64) {
	recordChannelFinancialConsume(ctx, relayInfo, modelName, quota, baseQuotaBeforeGroup, "")
}

// RecordChannelFinancialConsumeWithReference records a consume event while
// retaining a second durable identifier for asynchronous task callbacks.
// The request ID remains the gateway request ID so historical log matching is
// stable; the reference ID is used by task refunds to recover the snapshot.
func RecordChannelFinancialConsumeWithReference(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string, quota int, baseQuotaBeforeGroup *float64, referenceID string) {
	recordChannelFinancialConsume(ctx, relayInfo, modelName, quota, baseQuotaBeforeGroup, referenceID)
}

func recordChannelFinancialConsume(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string, quota int, baseQuotaBeforeGroup *float64, referenceID string) {
	if relayInfo == nil || relayInfo.ChannelMeta == nil || relayInfo.IsChannelTest ||
		!isFiniteNonNegative(common.QuotaPerUnit) || common.QuotaPerUnit <= 0 {
		return
	}
	costRate := relayInfo.ChannelCostRate
	if !model.IsValidChannelCostRate(costRate) {
		return
	}
	if quota < 0 {
		return
	}
	baseQuota, covered := financialBaseQuota(quota, relayInfo.PriceData, baseQuotaBeforeGroup)
	baseCostUSD := baseQuota / common.QuotaPerUnit
	channelCostUSD := baseCostUSD * costRate
	revenueUSD := float64(quota) / common.QuotaPerUnit
	if !isFiniteNonNegative(revenueUSD) {
		revenueUSD = 0
		covered = false
	}
	if !covered || !isFiniteNonNegative(baseCostUSD) || !isFiniteNonNegative(channelCostUSD) {
		baseCostUSD = 0
		channelCostUSD = 0
		covered = false
	}
	requestID := relayInfo.RequestId
	if ctx != nil {
		if contextRequestID := ctx.GetString(common.RequestIdKey); contextRequestID != "" {
			requestID = contextRequestID
		}
	}
	record := &model.ChannelFinancialRecord{
		CreatedAt:           financialRequestTimestamp(relayInfo),
		EventType:           model.FinancialEventConsume,
		RequestId:           requestID,
		ReferenceId:         referenceID,
		ChannelId:           relayInfo.ChannelId,
		ChannelName:         relayInfo.ChannelName,
		ModelName:           modelName,
		CostRate:            costRate,
		CostRateVersionId:   relayInfo.ChannelCostRateVersionId,
		CostRateEffectiveAt: relayInfo.ChannelCostRateEffectiveAt,
		RevenueUSD:          revenueUSD,
		BaseCostUSD:         baseCostUSD,
		ChannelCostUSD:      channelCostUSD,
		Quota:               quota,
		QuotaPerUnit:        common.QuotaPerUnit,
		Covered:             covered,
	}
	if err := model.CreateChannelFinancialRecord(record); err != nil {
		common.SysError("failed to record channel financial consume: " + err.Error())
	}
}

func financialRequestTimestamp(relayInfo *relaycommon.RelayInfo) int64 {
	if relayInfo != nil && !relayInfo.StartTime.IsZero() {
		return relayInfo.StartTime.Unix()
	}
	return 0
}

func RecordChannelFinancialTaskAdjustment(task *model.Task, eventType string, quota int) {
	if task == nil || quota <= 0 {
		return
	}
	channel := &model.Channel{Id: task.ChannelId, CostRate: 1}
	if current, err := model.GetChannelById(task.ChannelId, false); err == nil {
		channel = current
	}
	groupRatio := 0.0
	quotaPerUnit := common.QuotaPerUnit
	costRate := channel.CostRate
	costRateVersionID := int64(0)
	costRateEffectiveAt := int64(0)
	costRateSnapshotSet := false
	if task.PrivateData.BillingContext != nil {
		groupRatio = task.PrivateData.BillingContext.GroupRatio
		if task.PrivateData.BillingContext.ChannelName != "" {
			channel.Name = task.PrivateData.BillingContext.ChannelName
		}
		if task.PrivateData.BillingContext.ChannelCostRateSet {
			costRate = task.PrivateData.BillingContext.ChannelCostRate
			costRateVersionID = task.PrivateData.BillingContext.ChannelCostRateVersionId
			costRateEffectiveAt = task.PrivateData.BillingContext.ChannelCostRateEffectiveAt
			costRateSnapshotSet = true
		}
		if task.PrivateData.BillingContext.QuotaPerUnit > 0 {
			quotaPerUnit = task.PrivateData.BillingContext.QuotaPerUnit
		}
	}
	if !costRateSnapshotSet && task.SubmitTime > 0 {
		resolvedSnapshot, err := model.GetChannelCostRateSnapshotAt(task.ChannelId, time.Unix(task.SubmitTime, 0), costRate)
		if err != nil {
			common.SysError("failed to resolve task channel financial cost rate: " + err.Error())
		} else {
			costRate = resolvedSnapshot.CostRate
			costRateVersionID = resolvedSnapshot.VersionId
			costRateEffectiveAt = resolvedSnapshot.EffectiveAt
		}
	}
	if !isFiniteNonNegative(quotaPerUnit) || quotaPerUnit <= 0 {
		return
	}
	if !model.IsValidChannelCostRate(costRate) {
		return
	}
	baseQuota := 0.0
	covered := groupRatio > 0 && !math.IsNaN(groupRatio) && !math.IsInf(groupRatio, 0)
	if covered {
		baseQuota = float64(quota) / groupRatio
	}
	revenueUSD := float64(quota) / quotaPerUnit
	revenueFinite := isFiniteNonNegative(revenueUSD)
	if !revenueFinite {
		revenueUSD = 0
		covered = false
	}
	if eventType == model.FinancialEventRefund {
		if revenueFinite {
			revenueUSD = -revenueUSD
			covered = true
		} else {
			revenueUSD = 0
			covered = false
		}
		baseQuota = 0
	}
	baseCostUSD := baseQuota / quotaPerUnit
	channelCostUSD := baseCostUSD * costRate
	if !covered || !isFiniteNonNegative(baseCostUSD) || !isFiniteNonNegative(channelCostUSD) {
		baseCostUSD = 0
		channelCostUSD = 0
		covered = false
	}
	record := &model.ChannelFinancialRecord{
		EventType:           eventType,
		RequestId:           task.TaskID,
		ChannelId:           task.ChannelId,
		ChannelName:         channel.Name,
		ModelName:           taskModelName(task),
		CostRate:            costRate,
		CostRateVersionId:   costRateVersionID,
		CostRateEffectiveAt: costRateEffectiveAt,
		RevenueUSD:          revenueUSD,
		BaseCostUSD:         baseCostUSD,
		ChannelCostUSD:      channelCostUSD,
		Quota:               quota,
		QuotaPerUnit:        quotaPerUnit,
		Covered:             covered,
	}
	if err := model.CreateChannelFinancialRecord(record); err != nil {
		common.SysError("failed to record channel financial task adjustment: " + err.Error())
	}
}

func RecordChannelFinancialRefund(requestID string, channelID int, modelName string, quota int) {
	if quota <= 0 {
		return
	}
	quotaPerUnit := 0.0
	if isFiniteNonNegative(common.QuotaPerUnit) && common.QuotaPerUnit > 0 {
		quotaPerUnit = common.QuotaPerUnit
	}
	channel := &model.Channel{Id: channelID, CostRate: 1}
	costRateVersionID := int64(0)
	costRateEffectiveAt := int64(0)
	if snapshot, err := getFinancialConsumeSnapshot(requestID, channelID, modelName); err == nil {
		channel.Name = snapshot.ChannelName
		channel.CostRate = snapshot.CostRate
		costRateVersionID = snapshot.CostRateVersionId
		costRateEffectiveAt = snapshot.CostRateEffectiveAt
		if isFiniteNonNegative(snapshot.QuotaPerUnit) && snapshot.QuotaPerUnit > 0 {
			quotaPerUnit = snapshot.QuotaPerUnit
		}
	} else if current, err := model.GetChannelById(channelID, false); err == nil {
		channel = current
		if rateSnapshot, resolveErr := model.GetCurrentChannelCostRateSnapshot(channelID, current.CostRate); resolveErr != nil {
			common.SysError("failed to resolve refund channel financial cost rate: " + resolveErr.Error())
		} else {
			channel.CostRate = rateSnapshot.CostRate
			costRateVersionID = rateSnapshot.VersionId
			costRateEffectiveAt = rateSnapshot.EffectiveAt
		}
	}
	if !isFiniteNonNegative(quotaPerUnit) || quotaPerUnit <= 0 {
		return
	}
	if !model.IsValidChannelCostRate(channel.CostRate) {
		return
	}
	revenueUSD := float64(quota) / quotaPerUnit
	if !isFiniteNonNegative(revenueUSD) {
		return
	}
	record := &model.ChannelFinancialRecord{
		EventType:           model.FinancialEventRefund,
		RequestId:           requestID,
		ChannelId:           channelID,
		ChannelName:         channel.Name,
		ModelName:           modelName,
		CostRate:            channel.CostRate,
		CostRateVersionId:   costRateVersionID,
		CostRateEffectiveAt: costRateEffectiveAt,
		RevenueUSD:          -revenueUSD,
		Quota:               quota,
		QuotaPerUnit:        quotaPerUnit,
		Covered:             true,
	}
	if err := model.CreateChannelFinancialRecord(record); err != nil {
		common.SysError("failed to record channel financial refund: " + err.Error())
	}
}

func getFinancialConsumeSnapshot(requestID string, channelID int, modelName string) (*model.ChannelFinancialRecord, error) {
	if requestID == "" || model.DB == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var record model.ChannelFinancialRecord
	err := model.DB.Where("event_type = ? AND (request_id = ? OR reference_id = ?) AND channel_id = ? AND model_name = ?", model.FinancialEventConsume, requestID, requestID, channelID, modelName).
		Order("id DESC").First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func financialBaseQuota(quota int, priceData types.PriceData, exactBaseQuota *float64) (float64, bool) {
	if exactBaseQuota != nil && *exactBaseQuota >= 0 && !math.IsNaN(*exactBaseQuota) && !math.IsInf(*exactBaseQuota, 0) {
		return *exactBaseQuota, true
	}
	groupRatio := priceData.GroupRatioInfo.GroupRatio
	if math.IsNaN(groupRatio) || math.IsInf(groupRatio, 0) {
		return 0, false
	}
	if groupRatio == 0 {
		if priceData.UsePrice && priceData.ModelPrice >= 0 && !math.IsNaN(priceData.ModelPrice) && !math.IsInf(priceData.ModelPrice, 0) {
			baseQuota := priceData.ApplyOtherRatiosToFloat(priceData.ModelPrice * common.QuotaPerUnit)
			if isFiniteNonNegative(baseQuota) {
				return baseQuota, true
			}
		}
		return 0, false
	}
	if groupRatio < 0 {
		return 0, false
	}
	baseQuota := float64(quota) / groupRatio
	if !isFiniteNonNegative(baseQuota) {
		return 0, false
	}
	return baseQuota, true
}

func textFinancialBaseQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, summary textQuotaSummary, tieredResult *billingexpr.TieredResult) *float64 {
	if tieredResult != nil {
		baseQuota := tieredResult.ActualQuotaBeforeGroup
		if !summary.ToolCallSurchargeQuota.IsZero() && summary.GroupRatio != 0 {
			baseQuota += summary.ToolCallSurchargeQuota.Div(decimal.NewFromFloat(summary.GroupRatio)).InexactFloat64()
		}
		return &baseQuota
	}
	if summary.GroupRatio < 0 || math.IsNaN(summary.GroupRatio) || math.IsInf(summary.GroupRatio, 0) || !summary.hasBillableUsage() {
		return nil
	}
	baseRelayInfo := *relayInfo
	baseRelayInfo.PriceData = relayInfo.PriceData
	baseRelayInfo.PriceData.GroupRatioInfo.GroupRatio = 1
	baseSummary := calculateTextQuotaSummary(ctx, &baseRelayInfo, usage)
	baseQuota := float64(baseSummary.Quota)
	return &baseQuota
}

func audioFinancialBaseQuota(info QuotaInfo, modelPrice float64, totalTokens int) *float64 {
	if totalTokens <= 0 {
		return nil
	}
	if info.UsePrice && !isFiniteNonNegative(modelPrice) {
		return nil
	}
	info.ModelPrice = modelPrice
	info.GroupRatio = 1
	baseQuota, _ := calculateAudioQuota(info)
	baseQuotaValue := float64(baseQuota)
	if !isFiniteNonNegative(baseQuotaValue) {
		return nil
	}
	return &baseQuotaValue
}
