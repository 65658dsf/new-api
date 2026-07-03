package model

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	InvoiceStatusPending  = "pending"
	InvoiceStatusApproved = "approved"
	InvoiceStatusRejected = "rejected"
)

type InvoiceApplication struct {
	Id           int                        `json:"id"`
	UserId       int                        `json:"user_id" gorm:"index;not null"`
	TopUpId      int                        `json:"topup_id" gorm:"index;not null"`
	TradeNo      string                     `json:"trade_no" gorm:"type:varchar(255);uniqueIndex;not null"`
	Amount       int64                      `json:"amount"`
	Money        float64                    `json:"money"`
	Title        string                     `json:"title" gorm:"type:varchar(255);not null"`
	TaxId        string                     `json:"tax_id" gorm:"type:varchar(64);not null;index"`
	BuyerAddress string                     `json:"buyer_address" gorm:"type:varchar(255)"`
	BuyerPhone   string                     `json:"buyer_phone" gorm:"type:varchar(64)"`
	BankName     string                     `json:"bank_name" gorm:"type:varchar(255)"`
	BankAccount  string                     `json:"bank_account" gorm:"type:varchar(128)"`
	Status       string                     `json:"status" gorm:"type:varchar(20);index;not null"`
	RejectReason string                     `json:"reject_reason" gorm:"type:text"`
	PdfPath      string                     `json:"-" gorm:"type:varchar(512)"`
	PdfFileName  string                     `json:"pdf_file_name,omitempty" gorm:"type:varchar(255)"`
	CreatedAt    int64                      `json:"created_at" gorm:"index"`
	UpdatedAt    int64                      `json:"updated_at"`
	HandledAt    int64                      `json:"handled_at"`
	HandlerId    int                        `json:"handler_id"`
	User         *TopUpUserInfo             `json:"user,omitempty" gorm:"-"`
	Orders       []*InvoiceApplicationOrder `json:"orders,omitempty" gorm:"-"`
	HasPdf       bool                       `json:"has_pdf" gorm:"-"`
}

type InvoiceApplicationOrder struct {
	Id                   int     `json:"id"`
	InvoiceApplicationId int     `json:"invoice_application_id" gorm:"index;not null"`
	UserId               int     `json:"user_id" gorm:"index;not null"`
	TopUpId              int     `json:"topup_id" gorm:"uniqueIndex;not null"`
	TradeNo              string  `json:"trade_no" gorm:"type:varchar(255);uniqueIndex;not null"`
	Amount               int64   `json:"amount"`
	Money                float64 `json:"money"`
	CreatedAt            int64   `json:"created_at"`
}

type InvoiceApplicationSubmit struct {
	TradeNo      string   `json:"trade_no"`
	TradeNos     []string `json:"trade_nos"`
	Title        string   `json:"title"`
	TaxId        string   `json:"tax_id"`
	BuyerAddress string   `json:"buyer_address"`
	BuyerPhone   string   `json:"buyer_phone"`
	BankName     string   `json:"bank_name"`
	BankAccount  string   `json:"bank_account"`
}

type InvoiceApplicationQueryOptions struct {
	Keyword string
	Status  string
}

type InvoiceTopUpRecord struct {
	TopUp
	InvoiceApplicationId int    `json:"invoice_application_id,omitempty"`
	InvoiceStatus        string `json:"invoice_status,omitempty"`
	InvoiceApplied       bool   `json:"invoice_applied"`
}

var (
	invoiceTaxIdPattern = regexp.MustCompile(`^(?:[0-9A-Z]{18}|[0-9A-Z]{15}|[0-9A-Z]{17}|[0-9A-Z]{20})$`)
	mobilePhonePattern  = regexp.MustCompile(`^1[3-9]\d{9}$`)
	landlinePattern     = regexp.MustCompile(`^0\d{2,3}-?\d{7,8}(?:-\d{1,6})?$`)
)

func normalizeInvoiceApplicationInput(input InvoiceApplicationSubmit) InvoiceApplicationSubmit {
	tradeNos := make([]string, 0, len(input.TradeNos))
	for _, tradeNo := range input.TradeNos {
		tradeNo = strings.TrimSpace(tradeNo)
		if tradeNo != "" {
			tradeNos = append(tradeNos, tradeNo)
		}
	}
	return InvoiceApplicationSubmit{
		TradeNo:      strings.TrimSpace(input.TradeNo),
		TradeNos:     tradeNos,
		Title:        strings.TrimSpace(input.Title),
		TaxId:        strings.ToUpper(strings.TrimSpace(input.TaxId)),
		BuyerAddress: strings.TrimSpace(input.BuyerAddress),
		BuyerPhone:   strings.TrimSpace(input.BuyerPhone),
		BankName:     strings.TrimSpace(input.BankName),
		BankAccount:  strings.TrimSpace(input.BankAccount),
	}
}

func invoiceApplicationTradeNos(input InvoiceApplicationSubmit) []string {
	source := input.TradeNos
	if len(source) == 0 && strings.TrimSpace(input.TradeNo) != "" {
		source = []string{input.TradeNo}
	}

	seen := make(map[string]struct{}, len(source))
	tradeNos := make([]string, 0, len(source))
	for _, tradeNo := range source {
		tradeNo = strings.TrimSpace(tradeNo)
		if tradeNo == "" {
			continue
		}
		if _, ok := seen[tradeNo]; ok {
			continue
		}
		seen[tradeNo] = struct{}{}
		tradeNos = append(tradeNos, tradeNo)
	}
	return tradeNos
}

func ValidateInvoiceApplicationInput(input InvoiceApplicationSubmit) error {
	input = normalizeInvoiceApplicationInput(input)
	if input.Title == "" {
		return errors.New("请填写发票抬头全称")
	}
	if input.TaxId == "" {
		return errors.New("请填写统一社会信用代码或纳税人识别号")
	}
	if !invoiceTaxIdPattern.MatchString(input.TaxId) {
		return errors.New("请填写有效的统一社会信用代码或纳税人识别号")
	}
	if input.BuyerPhone != "" {
		phone := strings.ReplaceAll(input.BuyerPhone, " ", "")
		if !mobilePhonePattern.MatchString(phone) && !landlinePattern.MatchString(phone) {
			return errors.New("请填写有效的手机号或固定电话号码")
		}
	}
	return nil
}

func markInvoicePdfReady(app *InvoiceApplication) {
	if app != nil {
		app.HasPdf = app.PdfPath != ""
	}
}

func invoiceApplicationSummary(topUps []TopUp, tradeNos []string) (TopUp, int64, float64) {
	topUpByTradeNo := make(map[string]TopUp, len(topUps))
	for _, topUp := range topUps {
		topUpByTradeNo[topUp.TradeNo] = topUp
	}

	var first TopUp
	var totalAmount int64
	var totalMoney float64
	for idx, tradeNo := range tradeNos {
		topUp := topUpByTradeNo[tradeNo]
		if idx == 0 {
			first = topUp
		}
		totalAmount += topUp.Amount
		totalMoney += topUp.Money
	}
	return first, totalAmount, totalMoney
}

func updateInvoiceApplicationForSubmit(app *InvoiceApplication, input InvoiceApplicationSubmit, topUps []TopUp, tradeNos []string, now int64) {
	firstTopUp, totalAmount, totalMoney := invoiceApplicationSummary(topUps, tradeNos)
	app.TopUpId = firstTopUp.Id
	app.TradeNo = firstTopUp.TradeNo
	app.Amount = totalAmount
	app.Money = totalMoney
	app.Title = input.Title
	app.TaxId = input.TaxId
	app.BuyerAddress = input.BuyerAddress
	app.BuyerPhone = input.BuyerPhone
	app.BankName = input.BankName
	app.BankAccount = input.BankAccount
	app.Status = InvoiceStatusPending
	app.RejectReason = ""
	app.PdfPath = ""
	app.PdfFileName = ""
	app.CreatedAt = now
	app.UpdatedAt = now
	app.HandledAt = 0
	app.HandlerId = 0
}

func createInvoiceApplicationOrders(tx *gorm.DB, app *InvoiceApplication, topUps []TopUp, tradeNos []string, now int64) error {
	topUpByTradeNo := make(map[string]TopUp, len(topUps))
	for _, topUp := range topUps {
		topUpByTradeNo[topUp.TradeNo] = topUp
	}

	orders := make([]*InvoiceApplicationOrder, 0, len(tradeNos))
	for _, tradeNo := range tradeNos {
		topUp := topUpByTradeNo[tradeNo]
		orders = append(orders, &InvoiceApplicationOrder{
			InvoiceApplicationId: app.Id,
			UserId:               app.UserId,
			TopUpId:              topUp.Id,
			TradeNo:              topUp.TradeNo,
			Amount:               topUp.Amount,
			Money:                topUp.Money,
			CreatedAt:            now,
		})
	}
	if len(orders) == 0 {
		return nil
	}
	if err := tx.Create(&orders).Error; err != nil {
		return err
	}
	app.Orders = orders
	return nil
}

func replaceInvoiceApplicationOrders(tx *gorm.DB, app *InvoiceApplication, topUps []TopUp, tradeNos []string, now int64) error {
	if err := tx.Where("invoice_application_id = ?", app.Id).Delete(&InvoiceApplicationOrder{}).Error; err != nil {
		return err
	}
	return createInvoiceApplicationOrders(tx, app, topUps, tradeNos, now)
}

func attachInvoiceOrders(apps []*InvoiceApplication) error {
	if len(apps) == 0 {
		return nil
	}

	appIds := make([]int, 0, len(apps))
	appById := make(map[int]*InvoiceApplication, len(apps))
	for _, app := range apps {
		if app == nil || app.Id <= 0 {
			continue
		}
		appIds = append(appIds, app.Id)
		appById[app.Id] = app
	}
	if len(appIds) == 0 {
		return nil
	}

	var orders []InvoiceApplicationOrder
	if err := DB.Where("invoice_application_id IN ?", appIds).Order("id asc").Find(&orders).Error; err != nil {
		return err
	}
	for _, order := range orders {
		if app, ok := appById[order.InvoiceApplicationId]; ok {
			orderCopy := order
			app.Orders = append(app.Orders, &orderCopy)
		}
	}
	for _, app := range apps {
		if app == nil || len(app.Orders) > 0 || app.TradeNo == "" {
			continue
		}
		app.Orders = []*InvoiceApplicationOrder{
			{
				InvoiceApplicationId: app.Id,
				UserId:               app.UserId,
				TopUpId:              app.TopUpId,
				TradeNo:              app.TradeNo,
				Amount:               app.Amount,
				Money:                app.Money,
				CreatedAt:            app.CreatedAt,
			},
		}
	}
	return nil
}

func markInvoicePdfReadyList(apps []*InvoiceApplication) {
	for _, app := range apps {
		markInvoicePdfReady(app)
	}
}

func SubmitInvoiceApplication(userId int, input InvoiceApplicationSubmit) (*InvoiceApplication, error) {
	input = normalizeInvoiceApplicationInput(input)
	tradeNos := invoiceApplicationTradeNos(input)
	if len(tradeNos) == 0 {
		return nil, errors.New("订单号不能为空")
	}
	input.TradeNo = tradeNos[0]
	if err := ValidateInvoiceApplicationInput(input); err != nil {
		return nil, err
	}

	app := &InvoiceApplication{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var topUps []TopUp
		if err := tx.Where("trade_no IN ? AND user_id = ?", tradeNos, userId).Find(&topUps).Error; err != nil {
			return err
		}
		if len(topUps) != len(tradeNos) {
			return errors.New("充值订单不存在")
		}
		for _, topUp := range topUps {
			if topUp.Status != common.TopUpStatusSuccess {
				return errors.New("只有支付成功的充值订单可以申请开票")
			}
		}

		var existingOrders []InvoiceApplicationOrder
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("trade_no IN ?", tradeNos).Find(&existingOrders).Error; err != nil {
			return err
		}
		if len(existingOrders) > 0 {
			appId := existingOrders[0].InvoiceApplicationId
			for _, order := range existingOrders {
				if order.InvoiceApplicationId != appId {
					return errors.New("该订单已提交过开票申请")
				}
			}
			if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", appId).First(app).Error; err != nil {
				return err
			}
			if app.UserId != userId {
				return errors.New("该订单已提交过开票申请")
			}
			if app.Status != InvoiceStatusRejected {
				return errors.New("该订单已提交过开票申请")
			}

			now := common.GetTimestamp()
			updateInvoiceApplicationForSubmit(app, input, topUps, tradeNos, now)
			if err := tx.Save(app).Error; err != nil {
				return err
			}
			return replaceInvoiceApplicationOrders(tx, app, topUps, tradeNos, now)
		}

		var legacyApps []InvoiceApplication
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("trade_no IN ?", tradeNos).Find(&legacyApps).Error; err != nil {
			return err
		}
		if len(legacyApps) > 0 {
			if len(legacyApps) != 1 {
				return errors.New("该订单已提交过开票申请")
			}
			*app = legacyApps[0]
			if app.UserId != userId || app.Status != InvoiceStatusRejected {
				return errors.New("该订单已提交过开票申请")
			}

			now := common.GetTimestamp()
			updateInvoiceApplicationForSubmit(app, input, topUps, tradeNos, now)
			if err := tx.Save(app).Error; err != nil {
				return err
			}
			return createInvoiceApplicationOrders(tx, app, topUps, tradeNos, now)
		}

		now := common.GetTimestamp()
		*app = InvoiceApplication{
			UserId:       userId,
			Title:        input.Title,
			TaxId:        input.TaxId,
			BuyerAddress: input.BuyerAddress,
			BuyerPhone:   input.BuyerPhone,
			BankName:     input.BankName,
			BankAccount:  input.BankAccount,
			Status:       InvoiceStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		updateInvoiceApplicationForSubmit(app, input, topUps, tradeNos, now)
		if err := tx.Create(app).Error; err != nil {
			return err
		}
		return createInvoiceApplicationOrders(tx, app, topUps, tradeNos, now)
	})
	if err != nil {
		return nil, err
	}
	markInvoicePdfReady(app)
	return app, nil
}

func GetUserInvoiceTopUpRecords(userId int, pageInfo *common.PageInfo, keyword string) (records []*InvoiceTopUpRecord, total int64, err error) {
	query := DB.Model(&TopUp{}).Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess)
	if strings.TrimSpace(keyword) != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var topUps []TopUp
	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topUps).Error; err != nil {
		return nil, 0, err
	}

	tradeNos := make([]string, 0, len(topUps))
	for _, topUp := range topUps {
		if topUp.TradeNo != "" {
			tradeNos = append(tradeNos, topUp.TradeNo)
		}
	}

	invoiceByTradeNo := map[string]InvoiceApplication{}
	if len(tradeNos) > 0 {
		var orders []InvoiceApplicationOrder
		if err = DB.Where("trade_no IN ?", tradeNos).Find(&orders).Error; err != nil {
			return nil, 0, err
		}
		appIds := make([]int, 0, len(orders))
		for _, order := range orders {
			appIds = append(appIds, order.InvoiceApplicationId)
		}
		if len(appIds) > 0 {
			var apps []InvoiceApplication
			if err = DB.Where("id IN ?", appIds).Find(&apps).Error; err != nil {
				return nil, 0, err
			}
			appById := make(map[int]InvoiceApplication, len(apps))
			for _, app := range apps {
				appById[app.Id] = app
			}
			for _, order := range orders {
				if app, ok := appById[order.InvoiceApplicationId]; ok {
					invoiceByTradeNo[order.TradeNo] = app
				}
			}
		}

		var apps []InvoiceApplication
		if err = DB.Where("trade_no IN ?", tradeNos).Find(&apps).Error; err != nil {
			return nil, 0, err
		}
		for _, app := range apps {
			if _, ok := invoiceByTradeNo[app.TradeNo]; !ok {
				invoiceByTradeNo[app.TradeNo] = app
			}
		}
	}

	records = make([]*InvoiceTopUpRecord, 0, len(topUps))
	for _, topUp := range topUps {
		record := &InvoiceTopUpRecord{TopUp: topUp}
		if app, ok := invoiceByTradeNo[topUp.TradeNo]; ok {
			record.InvoiceApplied = true
			record.InvoiceApplicationId = app.Id
			record.InvoiceStatus = app.Status
		}
		records = append(records, record)
	}
	return records, total, nil
}

func GetUserInvoiceApplications(userId int, pageInfo *common.PageInfo) (apps []*InvoiceApplication, total int64, err error) {
	query := DB.Model(&InvoiceApplication{}).Where("user_id = ?", userId)
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&apps).Error; err != nil {
		return nil, 0, err
	}
	markInvoicePdfReadyList(apps)
	if err = attachInvoiceOrders(apps); err != nil {
		return nil, 0, err
	}
	return apps, total, nil
}

func applyInvoiceApplicationQueryOptions(query *gorm.DB, options InvoiceApplicationQueryOptions) (*gorm.DB, error) {
	if options.Status != "" {
		query = query.Where("status = ?", options.Status)
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

	conditions := []string{
		"trade_no LIKE ? ESCAPE '!'",
		"title LIKE ? ESCAPE '!'",
		"tax_id LIKE ? ESCAPE '!'",
	}
	args := []interface{}{pattern, pattern, pattern}

	if id, err := strconv.Atoi(keyword); err == nil && id > 0 {
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
		common.SysError("failed to search invoice users: " + err.Error())
		return nil, errors.New("搜索开票申请失败")
	}
	if len(userIds) > 0 {
		conditions = append(conditions, "user_id IN ?")
		args = append(args, userIds)
	}
	var appIds []int
	if err := DB.Model(&InvoiceApplicationOrder{}).
		Where("trade_no LIKE ? ESCAPE '!'", pattern).
		Limit(searchTopUpCountHardLimit).
		Pluck("invoice_application_id", &appIds).Error; err != nil {
		common.SysError("failed to search invoice application orders: " + err.Error())
		return nil, errors.New("搜索开票申请失败")
	}
	if len(appIds) > 0 {
		conditions = append(conditions, "id IN ?")
		args = append(args, appIds)
	}

	return query.Where("("+strings.Join(conditions, " OR ")+")", args...), nil
}

func attachInvoiceUsers(apps []*InvoiceApplication) error {
	userIdsMap := make(map[int]struct{})
	for _, app := range apps {
		if app.UserId > 0 {
			userIdsMap[app.UserId] = struct{}{}
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

	for _, app := range apps {
		if user, ok := userMap[app.UserId]; ok {
			userCopy := user
			app.User = &userCopy
		}
	}
	return nil
}

func GetAllInvoiceApplications(pageInfo *common.PageInfo, options InvoiceApplicationQueryOptions) (apps []*InvoiceApplication, total int64, err error) {
	query := DB.Model(&InvoiceApplication{})
	query, err = applyInvoiceApplicationQueryOptions(query, options)
	if err != nil {
		return nil, 0, err
	}

	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&apps).Error; err != nil {
		return nil, 0, err
	}
	markInvoicePdfReadyList(apps)
	if err = attachInvoiceOrders(apps); err != nil {
		return nil, 0, err
	}
	if err = attachInvoiceUsers(apps); err != nil {
		return nil, 0, err
	}
	return apps, total, nil
}

func GetPendingInvoiceApplicationCount() (int64, error) {
	var count int64
	err := DB.Model(&InvoiceApplication{}).
		Where("status = ?", InvoiceStatusPending).
		Count(&count).Error
	return count, err
}

func GetInvoiceApplicationById(id int) (*InvoiceApplication, error) {
	if id <= 0 {
		return nil, errors.New("开票申请不存在")
	}
	app := &InvoiceApplication{}
	if err := DB.Where("id = ?", id).First(app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("开票申请不存在")
		}
		return nil, err
	}
	markInvoicePdfReady(app)
	if err := attachInvoiceOrders([]*InvoiceApplication{app}); err != nil {
		return nil, err
	}
	return app, nil
}

func GetUserInvoiceApplicationById(userId int, id int) (*InvoiceApplication, error) {
	if id <= 0 {
		return nil, errors.New("开票申请不存在")
	}
	app := &InvoiceApplication{}
	if err := DB.Where("id = ? AND user_id = ?", id, userId).First(app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("开票申请不存在")
		}
		return nil, err
	}
	markInvoicePdfReady(app)
	if err := attachInvoiceOrders([]*InvoiceApplication{app}); err != nil {
		return nil, err
	}
	return app, nil
}

func CancelUserInvoiceApplication(userId int, id int) error {
	if id <= 0 {
		return errors.New("开票申请不存在")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		app := &InvoiceApplication{}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ? AND user_id = ?", id, userId).First(app).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("开票申请不存在")
			}
			return err
		}
		if app.Status == InvoiceStatusApproved {
			return errors.New("已通过的开票申请不能撤销")
		}
		if err := tx.Where("invoice_application_id = ?", app.Id).Delete(&InvoiceApplicationOrder{}).Error; err != nil {
			return err
		}
		return tx.Delete(app).Error
	})
}

func DeleteInvoiceApplicationByAdmin(id int) (*InvoiceApplication, error) {
	if id <= 0 {
		return nil, errors.New("开票申请不存在")
	}

	app := &InvoiceApplication{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", id).First(app).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("开票申请不存在")
			}
			return err
		}
		if err := tx.Where("invoice_application_id = ?", app.Id).Delete(&InvoiceApplicationOrder{}).Error; err != nil {
			return err
		}
		return tx.Delete(app).Error
	})
	if err != nil {
		return nil, err
	}
	return app, nil
}

func ApproveInvoiceApplication(id int, adminId int, pdfFileName string, pdfPath string) (*InvoiceApplication, error) {
	if pdfPath == "" {
		return nil, errors.New("请上传发票 PDF 文件")
	}
	app := &InvoiceApplication{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", id).First(app).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("开票申请不存在")
			}
			return err
		}
		if app.Status != InvoiceStatusPending {
			return errors.New("只能处理待审核的开票申请")
		}

		now := common.GetTimestamp()
		app.Status = InvoiceStatusApproved
		app.RejectReason = ""
		app.PdfPath = pdfPath
		app.PdfFileName = pdfFileName
		app.HandlerId = adminId
		app.HandledAt = now
		app.UpdatedAt = now
		return tx.Save(app).Error
	})
	if err != nil {
		return nil, err
	}
	markInvoicePdfReady(app)
	if err := attachInvoiceOrders([]*InvoiceApplication{app}); err != nil {
		return nil, err
	}
	return app, nil
}

func RejectInvoiceApplication(id int, adminId int, reason string) (*InvoiceApplication, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, errors.New("请填写拒绝原因")
	}

	app := &InvoiceApplication{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", id).First(app).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("开票申请不存在")
			}
			return err
		}
		if app.Status != InvoiceStatusPending {
			return errors.New("只能处理待审核的开票申请")
		}

		now := common.GetTimestamp()
		app.Status = InvoiceStatusRejected
		app.RejectReason = reason
		app.HandlerId = adminId
		app.HandledAt = now
		app.UpdatedAt = now
		return tx.Save(app).Error
	})
	if err != nil {
		return nil, err
	}
	markInvoicePdfReady(app)
	if err := attachInvoiceOrders([]*InvoiceApplication{app}); err != nil {
		return nil, err
	}
	return app, nil
}
