package controller

import (
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

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const invoicePDFMaxBytes int64 = 10 << 20

type rejectInvoiceApplicationRequest struct {
	Reason string `json:"reason"`
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

	app, err := model.SubmitInvoiceApplication(c.GetInt("id"), req)
	if err != nil {
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

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(apps)
	common.ApiSuccess(c, pageInfo)
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

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(apps)
	common.ApiSuccess(c, pageInfo)
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
