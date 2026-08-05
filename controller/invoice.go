package controller

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	invoicePDFMaxBytes              int64 = 10 << 20
	invoiceProviderListPageSize           = 100
	invoiceProviderListMaxPages           = 5
	invoiceProviderRecoveryMaxPages       = 5
	invoiceProviderRecoveryTimeout        = 10 * time.Second
	invoiceProviderSubmitTimeout          = 30 * time.Second
	invoiceProviderSyncTimeout            = 5 * time.Second
)

var errInvoiceProviderApplicationNotFound = errors.New("未找到对应的开票平台申请")

var invoiceProviderClientCache = struct {
	sync.Mutex
	clientID     string
	clientSecret string
	client       *service.InvoiceProviderClient
}{}

type rejectInvoiceApplicationRequest struct {
	Reason string `json:"reason"`
}

type invoicePendingCountResponse struct {
	PendingCount int64 `json:"pending_count"`
}

func GetUserInvoiceTopUpRecords(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	records, total, err := model.GetUserInvoiceTopUpRecords(userId, pageInfo, c.Query("keyword"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, pageInfo)
}

func SubmitInvoiceApplication(c *gin.Context) {
	var req model.InvoiceApplicationSubmit
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	providerClient, err := configuredInvoiceProviderClient()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(req.RecipientEmail) == "" {
		req.RecipientEmail = strings.TrimSpace(user.Email)
	}

	app, err := model.SubmitInvoiceApplication(user.Id, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	orderNos := make([]string, 0, len(app.Orders))
	for _, order := range app.Orders {
		if order != nil && strings.TrimSpace(order.TradeNo) != "" {
			orderNos = append(orderNos, order.TradeNo)
		}
	}
	submitCtx, cancel := context.WithTimeout(c.Request.Context(), invoiceProviderSubmitTimeout)
	defer cancel()
	validation, err := providerClient.ValidateOrders(submitCtx, orderNos)
	if err == nil {
		requestedOrderNos := make(map[string]struct{}, len(orderNos))
		for _, orderNo := range orderNos {
			requestedOrderNos[orderNo] = struct{}{}
		}
		if validation == nil || !invoiceProviderOrdersMatch(validation.Orders, requestedOrderNos) {
			err = errors.New("开票平台返回的订单校验结果不完整")
		}
	}
	if err == nil && !strings.EqualFold(strings.TrimSpace(validation.Currency), "CNY") {
		err = errors.New("开票平台返回了不支持的订单币种")
	}
	if err != nil {
		if recovered, recoveryErr := recoverExternalInvoiceApplication(c.Request.Context(), providerClient, app.Id, orderNos); recoveryErr == nil {
			common.ApiSuccess(c, recovered)
			return
		} else if !errors.Is(recoveryErr, errInvoiceProviderApplicationNotFound) {
			common.SysLog(fmt.Sprintf("failed to recover invoice application %d after validation error: %s", app.Id, recoveryErr.Error()))
		}
		markInvoiceApplicationSubmissionFailed(app.Id, "开票平台订单校验失败："+err.Error())
		common.ApiError(c, fmt.Errorf("开票平台订单校验失败: %w", err))
		return
	}

	externalInvoice, err := providerClient.CreateInvoice(submitCtx, service.InvoiceProviderCreateRequest{
		OrderNos:         orderNos,
		BuyerType:        app.BuyerType,
		Title:            app.Title,
		TaxpayerID:       app.TaxId,
		BuyerAddress:     app.BuyerAddress,
		BuyerPhone:       app.BuyerPhone,
		BuyerBank:        app.BankName,
		BuyerBankAccount: app.BankAccount,
		RecipientEmail:   app.RecipientEmail,
	})
	if err != nil {
		if recovered, recoveryErr := recoverExternalInvoiceApplication(c.Request.Context(), providerClient, app.Id, orderNos); recoveryErr == nil {
			common.ApiSuccess(c, recovered)
			return
		} else if !errors.Is(recoveryErr, errInvoiceProviderApplicationNotFound) {
			common.SysLog(fmt.Sprintf("failed to recover invoice application %d after provider submission error: %s", app.Id, recoveryErr.Error()))
		}
		markInvoiceApplicationSubmissionFailed(app.Id, "开票平台提交失败："+err.Error())
		common.ApiError(c, fmt.Errorf("开票平台提交失败: %w", err))
		return
	}
	app, err = bindExternalInvoiceApplication(app.Id, externalInvoice)
	if err != nil {
		if recovered, recoveryErr := recoverExternalInvoiceApplication(c.Request.Context(), providerClient, app.Id, orderNos); recoveryErr == nil {
			common.ApiSuccess(c, recovered)
			return
		} else if !errors.Is(recoveryErr, errInvoiceProviderApplicationNotFound) {
			common.SysLog(fmt.Sprintf("failed to recover invoice application %d after local binding error: %s", app.Id, recoveryErr.Error()))
		}
		markInvoiceApplicationSubmissionFailed(app.Id, "开票平台申请绑定失败："+err.Error())
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, app)
}

func GetUserInvoiceApplications(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	apps, total, err := model.GetUserInvoiceApplications(userId, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	syncExternalInvoiceApplications(c.Request.Context(), apps)

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(apps)
	common.ApiSuccess(c, pageInfo)
}

func CancelUserInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	if err := model.CancelUserInvoiceApplication(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func DownloadUserInvoicePDF(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	app, err := model.GetUserInvoiceApplicationById(c.GetInt("id"), id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if app.ExternalInvoiceId > 0 {
		providerClient, err := configuredInvoiceProviderClient()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		response, err := providerClient.DownloadPDF(c.Request.Context(), app.ExternalInvoiceId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		defer response.Body.Close()
		if response.ContentLength > invoicePDFMaxBytes {
			common.ApiErrorMsg(c, "发票 PDF 文件不能超过 10MB")
			return
		}

		if _, err := model.SyncExternalInvoiceApplication(app.Id, model.InvoiceStatusCompleted, app.ReviewNote, true); err != nil {
			common.SysLog(fmt.Sprintf("failed to cache completed invoice application %d: %s", app.Id, err.Error()))
		}
		fileName := app.PdfFileName
		if fileName == "" {
			fileName = fmt.Sprintf("invoice-%d.pdf", app.Id)
		}
		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
		if response.ContentLength >= 0 {
			c.Header("Content-Length", strconv.FormatInt(response.ContentLength, 10))
		}
		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, response.Body); err != nil {
			common.SysLog(fmt.Sprintf("failed to stream invoice application %d PDF: %s", app.Id, err.Error()))
		}
		return
	}
	if app.Status != model.InvoiceStatusApproved || app.PdfPath == "" {
		common.ApiErrorMsg(c, "发票 PDF 尚不可下载")
		return
	}

	absPath, err := invoicePDFAbsolutePath(app.PdfPath)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			common.ApiErrorMsg(c, "发票 PDF 文件不存在")
			return
		}
		common.ApiError(c, err)
		return
	}

	fileName := app.PdfFileName
	if fileName == "" {
		fileName = fmt.Sprintf("invoice-%d.pdf", app.Id)
	}
	c.Header("Content-Type", "application/pdf")
	c.FileAttachment(absPath, fileName)
}

func GetAllInvoiceApplications(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	options := model.InvoiceApplicationQueryOptions{
		Keyword: c.Query("keyword"),
		Status:  c.Query("status"),
	}

	apps, total, err := model.GetAllInvoiceApplications(pageInfo, options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	syncExternalInvoiceApplications(c.Request.Context(), apps)

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(apps)
	common.ApiSuccess(c, pageInfo)
}

func configuredInvoiceProviderClient() (*service.InvoiceProviderClient, error) {
	invoiceSettings := operation_setting.GetPaymentSetting()
	clientID := strings.TrimSpace(invoiceSettings.InvoiceClientID)
	clientSecret := strings.TrimSpace(invoiceSettings.InvoiceClientSecret)
	if clientID == "" || clientSecret == "" {
		return nil, errors.New("开票服务未配置，请联系管理员")
	}

	invoiceProviderClientCache.Lock()
	defer invoiceProviderClientCache.Unlock()
	if invoiceProviderClientCache.client != nil &&
		invoiceProviderClientCache.clientID == clientID &&
		invoiceProviderClientCache.clientSecret == clientSecret {
		return invoiceProviderClientCache.client, nil
	}
	invoiceProviderClientCache.clientID = clientID
	invoiceProviderClientCache.clientSecret = clientSecret
	invoiceProviderClientCache.client = service.NewInvoiceProviderClient(clientID, clientSecret)
	return invoiceProviderClientCache.client, nil
}

func markInvoiceApplicationSubmissionFailed(id int, reason string) {
	if err := model.MarkInvoiceApplicationSubmissionFailed(id, reason); err != nil {
		common.SysLog(fmt.Sprintf("failed to mark invoice application %d submission as failed: %s", id, err.Error()))
	}
}

func bindExternalInvoiceApplication(id int, externalInvoice *service.InvoiceProviderInvoice) (*model.InvoiceApplication, error) {
	if externalInvoice == nil {
		return nil, errors.New("开票平台返回的申请无效")
	}
	app, err := model.BindExternalInvoiceApplication(
		id,
		externalInvoice.ID,
		externalInvoice.Status,
		externalInvoice.ReviewNote,
	)
	if !errors.Is(err, model.ErrUnknownInvoiceStatus) {
		return app, err
	}

	status := strings.TrimSpace(externalInvoice.Status)
	if len(status) > 64 {
		status = status[:64]
	}
	common.SysLog(fmt.Sprintf("invoice provider application %d returned unknown status %q; caching it as pending", externalInvoice.ID, status))
	return model.BindExternalInvoiceApplication(
		id,
		externalInvoice.ID,
		model.InvoiceStatusPending,
		externalInvoice.ReviewNote,
	)
}

func recoverExternalInvoiceApplication(ctx context.Context, providerClient *service.InvoiceProviderClient, id int, orderNos []string) (*model.InvoiceApplication, error) {
	if providerClient == nil || id <= 0 || len(orderNos) == 0 {
		return nil, errInvoiceProviderApplicationNotFound
	}
	requestedOrderNos := make(map[string]struct{}, len(orderNos))
	for _, orderNo := range orderNos {
		orderNo = strings.TrimSpace(orderNo)
		if orderNo == "" {
			return nil, errInvoiceProviderApplicationNotFound
		}
		requestedOrderNos[orderNo] = struct{}{}
	}
	if len(requestedOrderNos) != len(orderNos) {
		return nil, errInvoiceProviderApplicationNotFound
	}

	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), invoiceProviderRecoveryTimeout)
	defer cancel()
	for page := 1; page <= invoiceProviderRecoveryMaxPages; page++ {
		remotePage, err := providerClient.ListInvoices(recoveryCtx, page, invoiceProviderListPageSize)
		if err != nil {
			return nil, err
		}
		for idx := range remotePage.Items {
			remote := &remotePage.Items[idx]
			if invoiceProviderOrderNosMatch(remote, requestedOrderNos) {
				return bindExternalInvoiceApplication(id, remote)
			}
		}
		if len(remotePage.Items) < invoiceProviderListPageSize ||
			(remotePage.Total > 0 && int64(page*invoiceProviderListPageSize) >= remotePage.Total) {
			break
		}
	}
	return nil, errInvoiceProviderApplicationNotFound
}

func invoiceProviderOrderNosMatch(invoice *service.InvoiceProviderInvoice, requested map[string]struct{}) bool {
	if invoice == nil || invoice.ID <= 0 || len(requested) == 0 {
		return false
	}
	if len(invoice.OrderNos) == len(requested) {
		matched := make(map[string]struct{}, len(invoice.OrderNos))
		for _, orderNo := range invoice.OrderNos {
			orderNo = strings.TrimSpace(orderNo)
			if _, ok := requested[orderNo]; !ok {
				matched = nil
				break
			}
			matched[orderNo] = struct{}{}
		}
		if len(matched) == len(requested) {
			return true
		}
	}

	return invoiceProviderOrdersMatch(invoice.Orders, requested)
}

func invoiceProviderOrdersMatch(orders []service.InvoiceProviderOrder, requested map[string]struct{}) bool {
	if len(orders) != len(requested) || len(requested) == 0 {
		return false
	}
	matched := make(map[string]struct{}, len(orders))
	for _, order := range orders {
		matchedOrderNo := ""
		for _, orderNo := range []string{order.OrderNo, order.ExternalNo, order.ExternalOrderNo} {
			orderNo = strings.TrimSpace(orderNo)
			if _, ok := requested[orderNo]; !ok {
				continue
			}
			if matchedOrderNo != "" && matchedOrderNo != orderNo {
				return false
			}
			matchedOrderNo = orderNo
		}
		if matchedOrderNo == "" {
			return false
		}
		if _, duplicate := matched[matchedOrderNo]; duplicate {
			return false
		}
		matched[matchedOrderNo] = struct{}{}
	}
	return len(matched) == len(requested)
}

func syncExternalInvoiceApplications(ctx context.Context, apps []*model.InvoiceApplication) {
	byExternalID := make(map[int64][]*model.InvoiceApplication)
	for _, app := range apps {
		if app != nil && app.ExternalInvoiceId > 0 &&
			app.Status != model.InvoiceStatusRejected && app.Status != model.InvoiceStatusCompleted {
			byExternalID[app.ExternalInvoiceId] = append(byExternalID[app.ExternalInvoiceId], app)
		}
	}
	if len(byExternalID) == 0 {
		return
	}
	providerClient, err := configuredInvoiceProviderClient()
	if err != nil {
		return
	}
	syncCtx, cancel := context.WithTimeout(ctx, invoiceProviderSyncTimeout)
	defer cancel()

	for page := 1; page <= invoiceProviderListMaxPages && len(byExternalID) > 0; page++ {
		remotePage, err := providerClient.ListInvoices(syncCtx, page, invoiceProviderListPageSize)
		if err != nil {
			common.SysLog("failed to synchronize invoice provider applications: " + err.Error())
			return
		}
		for _, remote := range remotePage.Items {
			matchedApps, ok := byExternalID[remote.ID]
			if !ok {
				continue
			}
			for _, app := range matchedApps {
				synced, err := model.SyncExternalInvoiceApplication(app.Id, remote.Status, remote.ReviewNote, remote.HasPDF)
				if err != nil {
					common.SysLog(fmt.Sprintf("failed to synchronize invoice application %d: %s", app.Id, err.Error()))
					continue
				}
				app.Status = synced.Status
				app.RejectReason = synced.RejectReason
				app.ReviewNote = synced.ReviewNote
				app.HandledAt = synced.HandledAt
				app.UpdatedAt = synced.UpdatedAt
				app.HasPdf = synced.HasPdf
			}
			delete(byExternalID, remote.ID)
		}
		if len(remotePage.Items) < invoiceProviderListPageSize ||
			(remotePage.Total > 0 && int64(page*invoiceProviderListPageSize) >= remotePage.Total) {
			return
		}
	}
}

func GetAdminInvoicePendingCount(c *gin.Context) {
	count, err := model.GetPendingInvoiceApplicationCount()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, invoicePendingCountResponse{PendingCount: count})
}

func AdminApproveInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, invoicePDFMaxBytes+1024*1024)
	file, header, err := c.Request.FormFile("pdf")
	if err != nil {
		common.ApiErrorMsg(c, "请上传发票 PDF 文件")
		return
	}
	defer file.Close()

	storedName, err := saveUploadedInvoicePDF(id, header, file)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	downloadName := fmt.Sprintf("invoice-%d.pdf", id)
	app, err := model.ApproveInvoiceApplication(id, c.GetInt("id"), downloadName, storedName)
	if err != nil {
		_ = os.Remove(filepath.Join(invoicePDFDir(), storedName))
		common.ApiError(c, err)
		return
	}

	notifyInvoiceApplicationUser(app, true)
	common.ApiSuccess(c, app)
}

func AdminRejectInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	var req rejectInvoiceApplicationRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	app, err := model.RejectInvoiceApplication(id, c.GetInt("id"), req.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	notifyInvoiceApplicationUser(app, false)
	common.ApiSuccess(c, app)
}

func AdminDeleteInvoiceApplication(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	app, err := model.DeleteInvoiceApplicationByAdmin(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if app.PdfPath != "" {
		if err := deleteInvoicePDF(app.PdfPath); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, nil)
}

func invoicePDFDir() string {
	configured := strings.TrimSpace(os.Getenv("INVOICE_FILE_DIR"))
	if configured != "" {
		return configured
	}
	return filepath.Join("data", "invoices")
}

func invoicePDFAbsolutePath(storedName string) (string, error) {
	storedName = filepath.Base(strings.TrimSpace(storedName))
	if storedName == "." || storedName == "" {
		return "", errors.New("发票 PDF 文件路径无效")
	}
	return filepath.Join(invoicePDFDir(), storedName), nil
}

func deleteInvoicePDF(storedName string) error {
	absPath, err := invoicePDFAbsolutePath(storedName)
	if err != nil {
		return err
	}
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveUploadedInvoicePDF(applicationId int, header *multipart.FileHeader, file multipart.File) (string, error) {
	if header == nil {
		return "", errors.New("请上传发票 PDF 文件")
	}
	if header.Size > invoicePDFMaxBytes {
		return "", errors.New("发票 PDF 文件不能超过 10MB")
	}
	if strings.ToLower(filepath.Ext(header.Filename)) != ".pdf" {
		return "", errors.New("仅允许上传 PDF 格式文件")
	}

	head := make([]byte, 512)
	n, readErr := file.Read(head)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", readErr
	}
	head = head[:n]
	if !strings.HasPrefix(string(head), "%PDF-") {
		return "", errors.New("仅允许上传有效的 PDF 文件")
	}

	dir := invoicePDFDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	storedName := fmt.Sprintf("invoice-%d-%s.pdf", applicationId, common.GetUUID())
	absPath := filepath.Join(dir, storedName)
	out, err := os.OpenFile(absPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}

	written := int64(0)
	cleanup := true
	defer func() {
		_ = out.Close()
		if cleanup {
			_ = os.Remove(absPath)
		}
	}()

	if n > 0 {
		count, err := out.Write(head)
		if err != nil {
			return "", err
		}
		written += int64(count)
	}

	remaining := invoicePDFMaxBytes - written + 1
	copied, err := io.Copy(out, io.LimitReader(file, remaining))
	written += copied
	if err != nil {
		return "", err
	}
	if written > invoicePDFMaxBytes {
		return "", errors.New("发票 PDF 文件不能超过 10MB")
	}

	cleanup = false
	return storedName, nil
}

func notifyInvoiceApplicationUser(app *model.InvoiceApplication, approved bool) {
	if app == nil {
		return
	}
	user, err := model.GetUserById(app.UserId, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to load invoice notification user %d: %s", app.UserId, err.Error()))
		return
	}

	title := "开票申请审核结果"
	content := fmt.Sprintf("您的充值订单 %s 开票申请已通过，发票 PDF 已可下载。", html.EscapeString(app.TradeNo))
	if !approved {
		content = fmt.Sprintf(
			"您的充值订单 %s 开票申请已被拒绝。原因：%s",
			html.EscapeString(app.TradeNo),
			html.EscapeString(app.RejectReason),
		)
	}

	notification := dto.NewNotify(dto.NotifyTypeInvoice, title, content, nil)
	if err := service.NotifyUser(user.Id, user.Email, user.GetSetting(), notification); err != nil {
		common.SysLog(fmt.Sprintf("failed to notify user %d for invoice application %d: %s", user.Id, app.Id, err.Error()))
	}
}

func notifyInvoiceApplicationAdmins(app *model.InvoiceApplication) {
	if app == nil {
		return
	}

	var admins []model.User
	if err := model.DB.
		Select("id", "email", "setting").
		Where("status = ? AND role >= ?", common.UserStatusEnabled, common.RoleAdminUser).
		Find(&admins).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to query invoice notification admins: %s", err.Error()))
		return
	}

	orderItems := make([]string, 0, len(app.Orders))
	for _, order := range app.Orders {
		if order == nil || strings.TrimSpace(order.TradeNo) == "" {
			continue
		}
		orderItems = append(orderItems, fmt.Sprintf(
			"<li>%s：%.2f</li>",
			html.EscapeString(order.TradeNo),
			order.Money,
		))
	}
	if len(orderItems) == 0 && strings.TrimSpace(app.TradeNo) != "" {
		orderItems = append(orderItems, fmt.Sprintf(
			"<li>%s：%.2f</li>",
			html.EscapeString(app.TradeNo),
			app.Money,
		))
	}

	subject := "新的开票申请待处理"
	content := fmt.Sprintf(
		"<p>有新的开票申请待处理，请前往管理员后台的开票申请管理页面处理。</p>"+
			"<p>申请编号：%d</p>"+
			"<p>申请用户 ID：%d</p>"+
			"<p>发票抬头：%s</p>"+
			"<p>统一社会信用代码/纳税人识别号：%s</p>"+
			"<p>订单明细：</p><ul>%s</ul>"+
			"<p>总金额：%.2f</p>",
		app.Id,
		app.UserId,
		html.EscapeString(app.Title),
		html.EscapeString(app.TaxId),
		strings.Join(orderItems, ""),
		app.Money,
	)

	for _, admin := range admins {
		email := strings.TrimSpace(admin.GetSetting().NotificationEmail)
		if email == "" {
			email = strings.TrimSpace(admin.Email)
		}
		if email == "" {
			common.SysLog(fmt.Sprintf("admin user %d has no email, skip invoice notification", admin.Id))
			continue
		}
		if err := common.SendEmail(subject, email, content); err != nil {
			common.SysLog(fmt.Sprintf("failed to notify admin %d for invoice application %d: %s", admin.Id, app.Id, err.Error()))
		}
	}
}
