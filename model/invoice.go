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
	Id           int            `json:"id"`
	UserId       int            `json:"user_id" gorm:"index;not null"`
	TopUpId      int            `json:"topup_id" gorm:"index;not null"`
	TradeNo      string         `json:"trade_no" gorm:"type:varchar(255);uniqueIndex;not null"`
	Amount       int64          `json:"amount"`
	Money        float64        `json:"money"`
	Title        string         `json:"title" gorm:"type:varchar(255);not null"`
	TaxId        string         `json:"tax_id" gorm:"type:varchar(64);not null;index"`
	BuyerAddress string         `json:"buyer_address" gorm:"type:varchar(255)"`
	BuyerPhone   string         `json:"buyer_phone" gorm:"type:varchar(64)"`
	BankName     string         `json:"bank_name" gorm:"type:varchar(255)"`
	BankAccount  string         `json:"bank_account" gorm:"type:varchar(128)"`
	Status       string         `json:"status" gorm:"type:varchar(20);index;not null"`
	RejectReason string         `json:"reject_reason" gorm:"type:text"`
	PdfPath      string         `json:"-" gorm:"type:varchar(512)"`
	PdfFileName  string         `json:"pdf_file_name,omitempty" gorm:"type:varchar(255)"`
	CreatedAt    int64          `json:"created_at" gorm:"index"`
	UpdatedAt    int64          `json:"updated_at"`
	HandledAt    int64          `json:"handled_at"`
	HandlerId    int            `json:"handler_id"`
	User         *TopUpUserInfo `json:"user,omitempty" gorm:"-"`
	HasPdf       bool           `json:"has_pdf" gorm:"-"`
}

type InvoiceApplicationSubmit struct {
	TradeNo      string `json:"trade_no"`
	Title        string `json:"title"`
	TaxId        string `json:"tax_id"`
	BuyerAddress string `json:"buyer_address"`
	BuyerPhone   string `json:"buyer_phone"`
	BankName     string `json:"bank_name"`
	BankAccount  string `json:"bank_account"`
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
	return InvoiceApplicationSubmit{
		TradeNo:      strings.TrimSpace(input.TradeNo),
		Title:        strings.TrimSpace(input.Title),
		TaxId:        strings.ToUpper(strings.TrimSpace(input.TaxId)),
		BuyerAddress: strings.TrimSpace(input.BuyerAddress),
		BuyerPhone:   strings.TrimSpace(input.BuyerPhone),
		BankName:     strings.TrimSpace(input.BankName),
		BankAccount:  strings.TrimSpace(input.BankAccount),
	}
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

func markInvoicePdfReadyList(apps []*InvoiceApplication) {
	for _, app := range apps {
		markInvoicePdfReady(app)
	}
}

func SubmitInvoiceApplication(userId int, input InvoiceApplicationSubmit) (*InvoiceApplication, error) {
	input = normalizeInvoiceApplicationInput(input)
	if input.TradeNo == "" {
		return nil, errors.New("订单号不能为空")
	}
	if err := ValidateInvoiceApplicationInput(input); err != nil {
		return nil, err
	}

	app := &InvoiceApplication{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var topUp TopUp
		if err := tx.Where("trade_no = ? AND user_id = ?", input.TradeNo, userId).First(&topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}
		if topUp.Status != common.TopUpStatusSuccess {
			return errors.New("只有支付成功的充值订单可以申请开票")
		}

		var count int64
		if err := tx.Model(&InvoiceApplication{}).Where("trade_no = ?", input.TradeNo).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("该订单已提交过开票申请")
		}

		now := common.GetTimestamp()
		*app = InvoiceApplication{
			UserId:       userId,
			TopUpId:      topUp.Id,
			TradeNo:      topUp.TradeNo,
			Amount:       topUp.Amount,
			Money:        topUp.Money,
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
		return tx.Create(app).Error
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
		var apps []InvoiceApplication
		if err = DB.Where("trade_no IN ?", tradeNos).Find(&apps).Error; err != nil {
			return nil, 0, err
		}
		for _, app := range apps {
			invoiceByTradeNo[app.TradeNo] = app
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
	if err = attachInvoiceUsers(apps); err != nil {
		return nil, 0, err
	}
	return apps, total, nil
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
	return app, nil
}
