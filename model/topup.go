package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
}

type TopUpUserInfo struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type TopUpRecord struct {
	TopUp
	User *TopUpUserInfo `json:"user,omitempty"`
}

type TopUpQueryOptions struct {
	Keyword         string
	Status          string
	PaymentMethod   string
	PaymentProvider string
	StartTime       int64
	EndTime         int64
}

type TopUpOverviewDaily struct {
	Date   string  `json:"date"`
	Income float64 `json:"income"`
	Orders int     `json:"orders"`
}

type TopUpOverviewPaymentMethod struct {
	PaymentMethod   string  `json:"payment_method"`
	PaymentProvider string  `json:"payment_provider"`
	Income          float64 `json:"income"`
	Orders          int     `json:"orders"`
}

type TopUpOverviewTopUser struct {
	User   TopUpUserInfo `json:"user"`
	Income float64       `json:"income"`
	Orders int           `json:"orders"`
}

type TopUpOverview struct {
	TodayIncome    float64                      `json:"today_income"`
	RangeIncome    float64                      `json:"range_income"`
	TodayOrders    int                          `json:"today_orders"`
	RangeOrders    int                          `json:"range_orders"`
	AverageAmount  float64                      `json:"average_amount"`
	Daily          []TopUpOverviewDaily         `json:"daily"`
	PaymentMethods []TopUpOverviewPaymentMethod `json:"payment_methods"`
	TopUsers       []TopUpOverviewTopUser       `json:"top_users"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch = errors.New("payment method mismatch")
	ErrTopUpNotFound         = errors.New("topup not found")
	ErrTopUpStatusInvalid    = errors.New("topup status invalid")
)

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.Status = targetStatus
		return tx.Save(topUp).Error
	})
}

func Recharge(referenceId string, customerId string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota float64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quota = topUp.Money * common.QuotaPerUnit
		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{"stripe_customer": customerId, "quota": gorm.Expr("quota + ?", quota)}).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(int(quota)), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

func applyTopUpQueryOptions(query *gorm.DB, options TopUpQueryOptions) (*gorm.DB, error) {
	if options.Status != "" {
		query = query.Where("status = ?", options.Status)
	}
	if options.PaymentMethod != "" {
		query = query.Where("payment_method = ?", options.PaymentMethod)
	}
	if options.PaymentProvider != "" {
		query = query.Where("payment_provider = ?", options.PaymentProvider)
	}
	if options.StartTime > 0 {
		query = query.Where("create_time >= ?", options.StartTime)
	}
	if options.EndTime > 0 {
		query = query.Where("create_time <= ?", options.EndTime)
	}
	if strings.TrimSpace(options.Keyword) == "" {
		return query, nil
	}

	keyword := strings.TrimSpace(options.Keyword)
	pattern, err := sanitizeLikePattern(keyword)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(pattern, "%") && len([]rune(keyword)) >= 2 {
		pattern = "%" + pattern + "%"
	}

	conditions := []string{"trade_no LIKE ? ESCAPE '!'"}
	args := []interface{}{pattern}

	if id, err := strconv.Atoi(keyword); err == nil {
		conditions = append(conditions, "id = ?", "user_id = ?")
		args = append(args, id, id)
	}

	userQuery := DB.Unscoped().Model(&User{})
	userQuery = userQuery.Where(
		"username LIKE ? ESCAPE '!' OR email LIKE ? ESCAPE '!' OR display_name LIKE ? ESCAPE '!'",
		pattern,
		pattern,
		pattern,
	)
	var userIds []int
	if err := userQuery.Limit(searchTopUpCountHardLimit).Pluck("id", &userIds).Error; err != nil {
		common.SysError("failed to search topup users: " + err.Error())
		return nil, errors.New("搜索充值记录失败")
	}
	if len(userIds) > 0 {
		conditions = append(conditions, "user_id IN ?")
		args = append(args, userIds)
	}

	return query.Where("("+strings.Join(conditions, " OR ")+")", args...), nil
}

func attachTopUpUsers(records []*TopUpRecord) error {
	userIdsMap := make(map[int]struct{})
	for _, record := range records {
		if record.UserId > 0 {
			userIdsMap[record.UserId] = struct{}{}
		}
	}
	if len(userIdsMap) == 0 {
		return nil
	}

	userIds := make([]int, 0, len(userIdsMap))
	for id := range userIdsMap {
		userIds = append(userIds, id)
	}

	var users []User
	if err := DB.Unscoped().
		Select("id", "username", "display_name", "email").
		Where("id IN ?", userIds).
		Find(&users).Error; err != nil {
		return err
	}

	userMap := make(map[int]TopUpUserInfo, len(users))
	for _, user := range users {
		userMap[user.Id] = TopUpUserInfo{
			Id:          user.Id,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Email:       user.Email,
		}
	}

	for _, record := range records {
		if user, ok := userMap[record.UserId]; ok {
			userCopy := user
			record.User = &userCopy
		}
	}
	return nil
}

func GetAllTopUpRecords(pageInfo *common.PageInfo, options TopUpQueryOptions) (records []*TopUpRecord, total int64, err error) {
	query := DB.Model(&TopUp{})
	query, err = applyTopUpQueryOptions(query, options)
	if err != nil {
		return nil, 0, err
	}

	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var topups []TopUp
	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		return nil, 0, err
	}

	records = make([]*TopUpRecord, 0, len(topups))
	for _, topup := range topups {
		topupCopy := topup
		records = append(records, &TopUpRecord{TopUp: topupCopy})
	}
	if err = attachTopUpUsers(records); err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

func topUpEffectiveTime(topUp TopUp) int64 {
	if topUp.CompleteTime > 0 {
		return topUp.CompleteTime
	}
	return topUp.CreateTime
}

func GetTopUpOverview(days int, now time.Time) (*TopUpOverview, error) {
	if days != 7 && days != 30 && days != 90 {
		days = 30
	}
	if now.IsZero() {
		now = time.Now()
	}

	location := now.Location()
	todayStart := time.Date(now.In(location).Year(), now.In(location).Month(), now.In(location).Day(), 0, 0, 0, 0, location)
	rangeStart := todayStart.AddDate(0, 0, -(days - 1))
	rangeEnd := todayStart.AddDate(0, 0, 1).Add(-time.Second)

	var topups []TopUp
	if err := DB.Where("status = ?", common.TopUpStatusSuccess).
		Where("(complete_time >= ? AND complete_time <= ?) OR (complete_time = 0 AND create_time >= ? AND create_time <= ?)",
			rangeStart.Unix(),
			rangeEnd.Unix(),
			rangeStart.Unix(),
			rangeEnd.Unix(),
		).
		Find(&topups).Error; err != nil {
		return nil, err
	}

	dailyMap := make(map[string]*TopUpOverviewDaily, days)
	for i := 0; i < days; i++ {
		date := rangeStart.AddDate(0, 0, i).Format("2006-01-02")
		dailyMap[date] = &TopUpOverviewDaily{Date: date}
	}

	type paymentAggregate struct {
		paymentMethod   string
		paymentProvider string
		income          float64
		orders          int
	}
	type userAggregate struct {
		income float64
		orders int
	}

	paymentMap := make(map[string]*paymentAggregate)
	userMap := make(map[int]*userAggregate)
	todayEnd := todayStart.AddDate(0, 0, 1).Add(-time.Second).Unix()
	todayStartUnix := todayStart.Unix()

	overview := &TopUpOverview{}
	for _, topUp := range topups {
		effectiveTime := topUpEffectiveTime(topUp)
		if effectiveTime < rangeStart.Unix() || effectiveTime > rangeEnd.Unix() {
			continue
		}

		overview.RangeIncome += topUp.Money
		overview.RangeOrders++
		if effectiveTime >= todayStartUnix && effectiveTime <= todayEnd {
			overview.TodayIncome += topUp.Money
			overview.TodayOrders++
		}

		date := time.Unix(effectiveTime, 0).In(location).Format("2006-01-02")
		if daily, ok := dailyMap[date]; ok {
			daily.Income += topUp.Money
			daily.Orders++
		}

		paymentKey := topUp.PaymentProvider + "\x00" + topUp.PaymentMethod
		aggregate, ok := paymentMap[paymentKey]
		if !ok {
			aggregate = &paymentAggregate{
				paymentMethod:   topUp.PaymentMethod,
				paymentProvider: topUp.PaymentProvider,
			}
			paymentMap[paymentKey] = aggregate
		}
		aggregate.income += topUp.Money
		aggregate.orders++

		if topUp.UserId > 0 {
			userAgg, ok := userMap[topUp.UserId]
			if !ok {
				userAgg = &userAggregate{}
				userMap[topUp.UserId] = userAgg
			}
			userAgg.income += topUp.Money
			userAgg.orders++
		}
	}

	if overview.RangeOrders > 0 {
		overview.AverageAmount = overview.RangeIncome / float64(overview.RangeOrders)
	}

	overview.Daily = make([]TopUpOverviewDaily, 0, days)
	for i := 0; i < days; i++ {
		date := rangeStart.AddDate(0, 0, i).Format("2006-01-02")
		overview.Daily = append(overview.Daily, *dailyMap[date])
	}

	overview.PaymentMethods = make([]TopUpOverviewPaymentMethod, 0, len(paymentMap))
	for _, aggregate := range paymentMap {
		overview.PaymentMethods = append(overview.PaymentMethods, TopUpOverviewPaymentMethod{
			PaymentMethod:   aggregate.paymentMethod,
			PaymentProvider: aggregate.paymentProvider,
			Income:          aggregate.income,
			Orders:          aggregate.orders,
		})
	}
	sort.SliceStable(overview.PaymentMethods, func(i, j int) bool {
		if overview.PaymentMethods[i].Income == overview.PaymentMethods[j].Income {
			return overview.PaymentMethods[i].Orders > overview.PaymentMethods[j].Orders
		}
		return overview.PaymentMethods[i].Income > overview.PaymentMethods[j].Income
	})

	userIds := make([]int, 0, len(userMap))
	for id := range userMap {
		userIds = append(userIds, id)
	}
	users := make(map[int]TopUpUserInfo, len(userIds))
	if len(userIds) > 0 {
		var rows []User
		if err := DB.Unscoped().
			Select("id", "username", "display_name", "email").
			Where("id IN ?", userIds).
			Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, user := range rows {
			users[user.Id] = TopUpUserInfo{
				Id:          user.Id,
				Username:    user.Username,
				DisplayName: user.DisplayName,
				Email:       user.Email,
			}
		}
	}

	overview.TopUsers = make([]TopUpOverviewTopUser, 0, len(userMap))
	for userId, aggregate := range userMap {
		user := users[userId]
		if user.Id == 0 {
			user.Id = userId
		}
		overview.TopUsers = append(overview.TopUsers, TopUpOverviewTopUser{
			User:   user,
			Income: aggregate.income,
			Orders: aggregate.orders,
		})
	}
	sort.SliceStable(overview.TopUsers, func(i, j int) bool {
		if overview.TopUsers[i].Income == overview.TopUsers[j].Income {
			return overview.TopUsers[i].Orders > overview.TopUsers[j].Orders
		}
		return overview.TopUsers[i].Income > overview.TopUsers[j].Income
	})
	if len(overview.TopUsers) > 10 {
		overview.TopUsers = overview.TopUsers[:10]
	}

	return overview, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		// 幂等处理：已成功直接返回
		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		if topUp.PaymentProvider == PaymentProviderStripe {
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(decimal.NewFromFloat(topUp.Money).Mul(dQuotaPerUnit).IntPart())
		} else {
			dAmount := decimal.NewFromInt(topUp.Amount)
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		}
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		return nil
	})

	if err != nil {
		return err
	}

	// 事务外记录日志，避免阻塞
	RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	return nil
}
func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int64
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// Creem 直接使用 Amount 作为充值额度（整数）
		quota = topUp.Amount

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{
			"quota": gorm.Expr("quota + ?", quota),
		}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		err = tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updateFields).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)

	return nil
}

func RechargeWaffo(tradeNo string, callerIp string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil // 幂等：已成功直接返回
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		dAmount := decimal.NewFromInt(topUp.Amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func RechargeWaffoPancake(tradeNo string) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		quotaToAdd = int(decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart())
		if quotaToAdd <= 0 {
			return errors.New("无效的充值额度")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
	}

	return nil
}
