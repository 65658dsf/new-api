package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type ChannelFinancialRecord struct {
	Id             int64   `json:"id" gorm:"primaryKey"`
	CreatedAt      int64   `json:"created_at" gorm:"bigint;index"`
	EventType      string  `json:"event_type" gorm:"type:varchar(16);index"`
	RequestId      string  `json:"request_id" gorm:"type:varchar(64);index"`
	ReferenceId    string  `json:"reference_id,omitempty" gorm:"type:varchar(64);index"`
	ChannelId      int     `json:"channel_id" gorm:"index"`
	ChannelName    string  `json:"channel_name" gorm:"type:varchar(255)"`
	ModelName      string  `json:"model_name" gorm:"type:varchar(255);index"`
	CostRate       float64 `json:"cost_rate" gorm:"type:decimal(20,10);not null"`
	RevenueUSD     float64 `json:"revenue_usd" gorm:"type:decimal(30,12);not null"`
	BaseCostUSD    float64 `json:"base_cost_usd" gorm:"type:decimal(30,12);not null"`
	ChannelCostUSD float64 `json:"channel_cost_usd" gorm:"type:decimal(30,12);not null"`
	Quota          int     `json:"quota"`
	QuotaPerUnit   float64 `json:"quota_per_unit" gorm:"type:decimal(30,12);not null"`
	Estimated      bool    `json:"estimated" gorm:"not null"`
	Covered        bool    `json:"covered" gorm:"not null"`
}

// ChannelFinancialLaunchOptionKey stores the first timestamp at which exact
// financial records are expected to be written. Logs before this timestamp are
// eligible for the one-time historical estimate fallback; newer logs must have
// an exact record or remain uncovered.
const ChannelFinancialLaunchOptionKey = "internal.channel_financial_launch_at"

// InitializeChannelFinancialLaunchTime is called as part of the main database
// migration. The existing row is preserved so the rollout boundary stays stable
// across restarts and migrations.
func InitializeChannelFinancialLaunchTime() error {
	if DB == nil {
		return errors.New("main database is not initialized")
	}
	var option Option
	result := DB.Where(&Option{Key: ChannelFinancialLaunchOptionKey}).First(&option)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return DB.Create(&Option{
			Key:   ChannelFinancialLaunchOptionKey,
			Value: strconv.FormatInt(common.GetTimestamp(), 10),
		}).Error
	}
	return result.Error
}

// GetChannelFinancialLaunchTime returns zero when the database predates the
// metadata row. That fallback preserves read-only compatibility for tests and
// installations that have not run the current migration yet.
func GetChannelFinancialLaunchTime() int64 {
	if DB == nil {
		return 0
	}
	var option Option
	if err := DB.Where(&Option{Key: ChannelFinancialLaunchOptionKey}).First(&option).Error; err != nil {
		return 0
	}
	value, err := strconv.ParseInt(option.Value, 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

// BeforeCreate rejects invalid snapshots without changing an explicit zero
// rate, which is a valid administrator configuration.
func (r *ChannelFinancialRecord) BeforeCreate(tx *gorm.DB) error {
	if r.CostRate < 0 || math.IsNaN(r.CostRate) || math.IsInf(r.CostRate, 0) {
		return errors.New("channel financial cost rate must be a non-negative finite number")
	}
	if math.IsNaN(r.RevenueUSD) || math.IsInf(r.RevenueUSD, 0) {
		return errors.New("channel financial revenue must be finite")
	}
	if r.BaseCostUSD < 0 || math.IsNaN(r.BaseCostUSD) || math.IsInf(r.BaseCostUSD, 0) {
		return errors.New("channel financial base cost must be a non-negative finite number")
	}
	if r.ChannelCostUSD < 0 || math.IsNaN(r.ChannelCostUSD) || math.IsInf(r.ChannelCostUSD, 0) {
		return errors.New("channel financial channel cost must be a non-negative finite number")
	}
	if r.QuotaPerUnit < 0 || math.IsNaN(r.QuotaPerUnit) || math.IsInf(r.QuotaPerUnit, 0) {
		return errors.New("channel financial quota per unit must be a non-negative finite number")
	}
	if r.CreatedAt == 0 {
		r.CreatedAt = time.Now().Unix()
	}
	return nil
}

const (
	FinancialEventConsume = "consume"
	FinancialEventRefund  = "refund"
)

type FinancialSummary struct {
	RevenueUSD     float64 `json:"revenue_usd"`
	CostUSD        float64 `json:"cost_usd"`
	ProfitUSD      float64 `json:"profit_usd"`
	ProfitMargin   float64 `json:"profit_margin"`
	RequestCount   int64   `json:"request_count"`
	CoveredCount   int64   `json:"covered_count"`
	ExactCount     int64   `json:"exact_count"`
	EstimatedCount int64   `json:"estimated_count"`
	UncoveredCount int64   `json:"uncovered_count"`
}

type FinancialTrendPoint struct {
	Date         string  `json:"date"`
	RevenueUSD   float64 `json:"revenue_usd"`
	CostUSD      float64 `json:"cost_usd"`
	ProfitUSD    float64 `json:"profit_usd"`
	RequestCount int64   `json:"request_count"`
}

type FinancialDimensionRow struct {
	Id             int     `json:"id,omitempty"`
	Name           string  `json:"name"`
	ModelName      string  `json:"model_name,omitempty"`
	RevenueUSD     float64 `json:"revenue_usd"`
	CostUSD        float64 `json:"cost_usd"`
	ProfitUSD      float64 `json:"profit_usd"`
	RequestCount   int64   `json:"request_count"`
	CoveredCount   int64   `json:"covered_count"`
	EstimatedCount int64   `json:"estimated_count"`
	UncoveredCount int64   `json:"uncovered_count"`
}

type ChannelFinancialReport struct {
	Summary    FinancialSummary        `json:"summary"`
	Trend      []FinancialTrendPoint   `json:"trend"`
	ByChannel  []FinancialDimensionRow `json:"by_channel"`
	ByModel    []FinancialDimensionRow `json:"by_model"`
	RangeStart int64                   `json:"range_start"`
	RangeEnd   int64                   `json:"range_end"`
}

func CreateChannelFinancialRecord(record *ChannelFinancialRecord) error {
	if record == nil {
		return nil
	}
	if DB == nil {
		return errors.New("main database is not initialized")
	}
	return DB.Create(record).Error
}

func GetChannelFinancialRecords(start, end int64, channelID int, modelName string) ([]*ChannelFinancialRecord, error) {
	if DB == nil {
		return nil, errors.New("main database is not initialized")
	}
	query := DB.Model(&ChannelFinancialRecord{}).Where("created_at >= ? AND created_at <= ?", start, end)
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	var rows []*ChannelFinancialRecord
	return rows, query.Order("created_at ASC, id ASC").Find(&rows).Error
}

func GetHistoricalChannelFinancialRecords(start, end int64, channelID int, modelName string, exactRows []*ChannelFinancialRecord) ([]*ChannelFinancialRecord, error) {
	if LOG_DB == nil {
		return nil, nil
	}
	quotaPerUnit := common.QuotaPerUnit
	quotaPerUnitValid := isFiniteNonNegative(quotaPerUnit) && quotaPerUnit > 0
	if !quotaPerUnitValid {
		quotaPerUnit = 0
	}
	query := LOG_DB.Model(&Log{}).
		Where("created_at >= ? AND created_at <= ?", start, end).
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund}).
		Where("COALESCE(token_name, '') <> ?", "模型测试").
		Where("channel_id > 0")
	launchAt := GetChannelFinancialLaunchTime()
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	var logs []*Log
	if err := query.Order("created_at ASC").Find(&logs).Error; err != nil {
		return nil, err
	}

	exactKeys := make(map[string]struct{}, len(exactRows))
	for _, row := range exactRows {
		if row != nil {
			exactKeys[financialRecordKey(row.RequestId, row.EventType, row.ChannelId, row.ModelName, row.Quota)] = struct{}{}
			if row.ReferenceId != "" {
				exactKeys[financialRecordKey(row.ReferenceId, row.EventType, row.ChannelId, row.ModelName, row.Quota)] = struct{}{}
			}
			exactKeys[financialRecordFallbackKey(row.CreatedAt, row.EventType, row.ChannelId, row.ModelName, row.Quota)] = struct{}{}
		}
	}
	channelIDs := make([]int, 0)
	seenChannelIDs := make(map[int]struct{})
	for _, log := range logs {
		if _, exists := seenChannelIDs[log.ChannelId]; exists {
			continue
		}
		seenChannelIDs[log.ChannelId] = struct{}{}
		channelIDs = append(channelIDs, log.ChannelId)
	}
	var channels []Channel
	if len(channelIDs) > 0 {
		if err := DB.Select("id", "name", "COALESCE(cost_rate, 1) AS cost_rate").Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
			return nil, err
		}
	}
	channelByID := make(map[int]Channel, len(channels))
	for _, channel := range channels {
		channelByID[channel.Id] = channel
	}

	rows := make([]*ChannelFinancialRecord, 0, len(logs))
	for _, log := range logs {
		eventType := FinancialEventConsume
		if log.Type == LogTypeRefund {
			eventType = FinancialEventRefund
		}
		if _, exists := exactKeys[financialRecordKey(log.RequestId, eventType, log.ChannelId, log.ModelName, log.Quota)]; exists {
			continue
		}
		if log.RequestId == "" {
			if _, exists := exactKeys[financialRecordFallbackKey(log.CreatedAt, eventType, log.ChannelId, log.ModelName, log.Quota)]; exists {
				continue
			}
		}
		channel, exists := channelByID[log.ChannelId]
		costRate := 1.0
		channelName := ""
		if exists {
			costRate = channel.CostRate
			channelName = channel.Name
		}
		if costRate < 0 || math.IsNaN(costRate) || math.IsInf(costRate, 0) {
			costRate = 1
		}
		revenueUSD := 0.0
		if quotaPerUnitValid {
			revenueUSD = float64(log.Quota) / quotaPerUnit
			if !isFiniteNonNegative(revenueUSD) {
				revenueUSD = 0
			}
		}
		row := &ChannelFinancialRecord{
			CreatedAt:    log.CreatedAt,
			EventType:    eventType,
			RequestId:    log.RequestId,
			ChannelId:    log.ChannelId,
			ChannelName:  channelName,
			ModelName:    log.ModelName,
			CostRate:     costRate,
			RevenueUSD:   revenueUSD,
			Quota:        log.Quota,
			QuotaPerUnit: quotaPerUnit,
			Estimated:    launchAt <= 0 || log.CreatedAt < launchAt,
		}
		if launchAt > 0 && log.CreatedAt >= launchAt {
			settled, known := financialLogSettlementStatus(log.Other)
			if !known {
				// A post-launch log without the settlement marker belongs in the
				// report as an uncovered event. We cannot safely infer either a
				// charge or an upstream cost from an unknown settlement outcome.
				row.RevenueUSD = 0
				row.BaseCostUSD = 0
				row.ChannelCostUSD = 0
				rows = append(rows, row)
				continue
			}
			if eventType == FinancialEventRefund {
				if settled && quotaPerUnitValid && log.Quota > 0 {
					row.RevenueUSD = -revenueUSD
					row.Covered = true
				} else {
					row.RevenueUSD = 0
				}
			} else if !settled {
				row.RevenueUSD = 0
			}
			rows = append(rows, row)
			continue
		}
		if eventType == FinancialEventConsume && log.Quota < 0 {
			row.RevenueUSD = 0
			rows = append(rows, row)
			continue
		}
		if eventType == FinancialEventRefund {
			if quotaPerUnitValid && log.Quota > 0 {
				row.RevenueUSD = -math.Abs(revenueUSD)
				row.Covered = true
			}
			rows = append(rows, row)
			continue
		}
		groupRatio, ok := financialLogGroupRatio(log.Other)
		if ok && log.Quota >= 0 && quotaPerUnitValid {
			baseCostUSD := float64(log.Quota) / groupRatio / quotaPerUnit
			channelCostUSD := baseCostUSD * costRate
			if isFiniteNonNegative(baseCostUSD) && isFiniteNonNegative(channelCostUSD) {
				row.BaseCostUSD = baseCostUSD
				row.ChannelCostUSD = channelCostUSD
				row.Covered = true
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func financialRecordKey(requestID, eventType string, channelID int, modelName string, quota int) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%d", requestID, eventType, channelID, modelName, quota)
}

func financialRecordFallbackKey(createdAt int64, eventType string, channelID int, modelName string, quota int) string {
	return fmt.Sprintf("%d\x00%s\x00%d\x00%s\x00%d", createdAt, eventType, channelID, modelName, quota)
}

func financialLogGroupRatio(other string) (float64, bool) {
	var data map[string]any
	if other == "" || common.UnmarshalJsonStr(other, &data) != nil {
		return 0, false
	}
	ratio, ok := data["group_ratio"].(float64)
	if !ok || ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return 0, false
	}
	return ratio, true
}

func financialLogSettlementStatus(other string) (bool, bool) {
	var data map[string]any
	if other == "" || common.UnmarshalJsonStr(other, &data) != nil {
		return false, false
	}
	settled, ok := data["financial_settled"].(bool)
	return settled, ok
}

func isFiniteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func SumChannelFinancialRecords(start, end int64, channelID int, modelName string) (FinancialSummary, error) {
	rows, err := GetChannelFinancialRecords(start, end, channelID, modelName)
	if err != nil {
		return FinancialSummary{}, err
	}
	report := BuildChannelFinancialReport(rows, start, end)
	return report.Summary, nil
}

func BuildChannelFinancialReport(rows []*ChannelFinancialRecord, start, end int64) ChannelFinancialReport {
	report := ChannelFinancialReport{
		RangeStart: start,
		RangeEnd:   end,
		Trend:      make([]FinancialTrendPoint, 0),
		ByChannel:  make([]FinancialDimensionRow, 0),
		ByModel:    make([]FinancialDimensionRow, 0),
	}
	trend := make(map[string]*FinancialTrendPoint)
	channels := make(map[int]*FinancialDimensionRow)
	models := make(map[string]*FinancialDimensionRow)
	for _, row := range rows {
		if row == nil {
			continue
		}
		normalized := *row
		if math.IsNaN(normalized.RevenueUSD) || math.IsInf(normalized.RevenueUSD, 0) {
			normalized.RevenueUSD = 0
			normalized.Covered = false
		}
		if !isFiniteNonNegative(normalized.ChannelCostUSD) {
			normalized.ChannelCostUSD = 0
			normalized.Covered = false
		}
		row = &normalized
		addFinancialSummary(&report.Summary, row)

		date := time.Unix(row.CreatedAt, 0).Local().Format("2006-01-02")
		point := trend[date]
		if point == nil {
			point = &FinancialTrendPoint{Date: date}
			trend[date] = point
		}
		point.RevenueUSD += row.RevenueUSD
		point.CostUSD += row.ChannelCostUSD
		point.ProfitUSD = point.RevenueUSD - point.CostUSD
		if row.EventType == FinancialEventConsume {
			point.RequestCount++
		}

		channel := channels[row.ChannelId]
		if channel == nil {
			channel = &FinancialDimensionRow{Id: row.ChannelId, Name: row.ChannelName}
			channels[row.ChannelId] = channel
		}
		addFinancialDimension(channel, row)

		modelRow := models[row.ModelName]
		if modelRow == nil {
			modelRow = &FinancialDimensionRow{Name: row.ModelName, ModelName: row.ModelName}
			models[row.ModelName] = modelRow
		}
		addFinancialDimension(modelRow, row)
	}
	finalizeFinancialSummary(&report.Summary)
	for _, point := range trend {
		report.Trend = append(report.Trend, *point)
	}
	for _, row := range channels {
		report.ByChannel = append(report.ByChannel, *row)
	}
	for _, row := range models {
		report.ByModel = append(report.ByModel, *row)
	}
	sort.Slice(report.Trend, func(i, j int) bool { return report.Trend[i].Date < report.Trend[j].Date })
	sort.Slice(report.ByChannel, func(i, j int) bool {
		if report.ByChannel[i].RevenueUSD == report.ByChannel[j].RevenueUSD {
			if report.ByChannel[i].Name == report.ByChannel[j].Name {
				return report.ByChannel[i].Id < report.ByChannel[j].Id
			}
			return report.ByChannel[i].Name < report.ByChannel[j].Name
		}
		return report.ByChannel[i].RevenueUSD > report.ByChannel[j].RevenueUSD
	})
	sort.Slice(report.ByModel, func(i, j int) bool {
		if report.ByModel[i].RevenueUSD == report.ByModel[j].RevenueUSD {
			return report.ByModel[i].ModelName < report.ByModel[j].ModelName
		}
		return report.ByModel[i].RevenueUSD > report.ByModel[j].RevenueUSD
	})
	return report
}

func addFinancialSummary(summary *FinancialSummary, row *ChannelFinancialRecord) {
	summary.RevenueUSD += row.RevenueUSD
	summary.CostUSD += row.ChannelCostUSD
	if row.EventType == FinancialEventConsume {
		summary.RequestCount++
	}
	if row.Covered {
		summary.CoveredCount++
	} else {
		summary.UncoveredCount++
	}
	if row.Estimated {
		summary.EstimatedCount++
	} else {
		summary.ExactCount++
	}
}

func finalizeFinancialSummary(summary *FinancialSummary) {
	summary.ProfitUSD = summary.RevenueUSD - summary.CostUSD
	if summary.RevenueUSD != 0 {
		summary.ProfitMargin = summary.ProfitUSD / summary.RevenueUSD
	}
}

func addFinancialDimension(dimension *FinancialDimensionRow, row *ChannelFinancialRecord) {
	dimension.RevenueUSD += row.RevenueUSD
	dimension.CostUSD += row.ChannelCostUSD
	dimension.ProfitUSD = dimension.RevenueUSD - dimension.CostUSD
	if row.EventType == FinancialEventConsume {
		dimension.RequestCount++
	}
	if row.Covered {
		dimension.CoveredCount++
	} else {
		dimension.UncoveredCount++
	}
	if row.Estimated {
		dimension.EstimatedCount++
	}
}
