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
}

// 默认配置
var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume: true,
	InviterReward: InviterRewardSetting{
		Mode: InviterRewardModeFixed,
	},
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
