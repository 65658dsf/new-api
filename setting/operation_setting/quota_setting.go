package operation_setting

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	InviterRewardModeFixed      = "fixed"
	InviterRewardModePercentage = "percentage"
)

type InviterRewardSetting struct {
	Mode       string  `json:"mode"`
	Percentage float64 `json:"percentage"`
}

type QuotaSetting struct {
	EnableFreeModelPreConsume bool                 `json:"enable_free_model_pre_consume"` // 是否对免费模型启用预消耗
	InviterReward             InviterRewardSetting `json:"inviter_reward"`
	TrustQuotaUSD             float64              `json:"trust_quota_usd"`        // 钱包免预扣门槛，0 表示禁用
	PreConsumeMultiplier      float64              `json:"pre_consume_multiplier"` // 预计输入费用的预扣倍率，仅影响预留
}

// 默认配置
var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume: true,
	InviterReward: InviterRewardSetting{
		Mode: InviterRewardModeFixed,
	},
	TrustQuotaUSD:        10,
	PreConsumeMultiplier: 1,
}

var quotaSettingMutex sync.RWMutex

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("quota_setting", &quotaSetting)
}

func GetQuotaSetting() QuotaSetting {
	quotaSettingMutex.RLock()
	defer quotaSettingMutex.RUnlock()
	return quotaSetting
}

func IsValidInviterRewardMode(mode string) bool {
	return mode == InviterRewardModeFixed || mode == InviterRewardModePercentage
}

func IsValidInviterRewardPercentage(percentage float64) bool {
	return !math.IsNaN(percentage) && !math.IsInf(percentage, 0) && percentage >= 0 && percentage <= 100
}

func GetInviterRewardSetting() InviterRewardSetting {
	reward := GetQuotaSetting().InviterReward
	if !IsValidInviterRewardMode(reward.Mode) {
		reward.Mode = InviterRewardModeFixed
	}
	if !IsValidInviterRewardPercentage(reward.Percentage) {
		reward.Percentage = 0
	}
	return reward
}

func UpdateQuotaSetting(values map[string]string) error {
	quotaSettingMutex.Lock()
	defer quotaSettingMutex.Unlock()

	next := quotaSetting
	if raw, ok := values["enable_free_model_pre_consume"]; ok {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid enable_free_model_pre_consume: %w", err)
		}
		next.EnableFreeModelPreConsume = enabled
	}
	for _, key := range []string{"trust_quota_usd", "pre_consume_multiplier"} {
		if raw, ok := values[key]; ok {
			if err := ValidateQuotaOption("quota_setting."+key, raw); err != nil {
				return err
			}
			number, _ := strconv.ParseFloat(raw, 64)
			if key == "trust_quota_usd" {
				next.TrustQuotaUSD = number
			} else {
				next.PreConsumeMultiplier = number
			}
		}
	}
	if raw, ok := values["inviter_reward"]; ok {
		var reward InviterRewardSetting
		if err := common.UnmarshalJsonStr(raw, &reward); err != nil {
			return fmt.Errorf("invalid inviter_reward: %w", err)
		}
		if !IsValidInviterRewardMode(reward.Mode) {
			return fmt.Errorf("invalid inviter reward mode: %s", reward.Mode)
		}
		if !IsValidInviterRewardPercentage(reward.Percentage) {
			return fmt.Errorf("invalid inviter reward percentage: %v", reward.Percentage)
		}
		next.InviterReward = reward
	}

	quotaSetting = next
	return nil
}

// ValidateQuotaOption validates reservation settings before they are persisted.
func ValidateQuotaOption(key, value string) error {
	if key != "quota_setting.trust_quota_usd" && key != "quota_setting.pre_consume_multiplier" {
		return nil
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return fmt.Errorf("%s must be a finite non-negative number", key)
	}
	if key == "quota_setting.pre_consume_multiplier" && number == 0 {
		return fmt.Errorf("%s must be greater than zero", key)
	}
	return nil
}

// InputPreConsumeMultiplier rejects invalid runtime settings as well as invalid saves.
func InputPreConsumeMultiplier() (float64, error) {
	multiplier := GetQuotaSetting().PreConsumeMultiplier
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0, fmt.Errorf("pre-consume multiplier must be a finite number greater than zero")
	}
	return multiplier, nil
}
