package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createInvoiceTestUser(t *testing.T, suffix string) *User {
	t.Helper()

	user := &User{
		Username:    "invoice_user_" + suffix,
		Password:    "password_hash",
		DisplayName: "Invoice User " + suffix,
		Email:       "invoice_" + suffix + "@example.com",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "aff_" + suffix,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func createInvoiceTestTopUp(t *testing.T, userId int, tradeNo string, status string) *TopUp {
	t.Helper()

	now := common.GetTimestamp()
	topUp := &TopUp{
		UserId:          userId,
		Amount:          100,
		Money:           10,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		CreateTime:      now - 60,
		CompleteTime:    now,
		Status:          status,
	}
	require.NoError(t, DB.Create(topUp).Error)
	return topUp
}

func validInvoiceSubmit(tradeNo string) InvoiceApplicationSubmit {
	return InvoiceApplicationSubmit{
		TradeNo:      tradeNo,
		Title:        "Example Technology Co Ltd",
		TaxId:        "91350211M000100Y43",
		BuyerAddress: "123 Example Road",
		BuyerPhone:   "010-12345678",
		BankName:     "Example Bank",
		BankAccount:  "6222000000000000",
	}
}

func TestSubmitInvoiceApplicationCreatesPendingApplication(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "submit")
	topUp := createInvoiceTestTopUp(t, user.Id, "invoice_submit_success", common.TopUpStatusSuccess)

	app, err := SubmitInvoiceApplication(user.Id, validInvoiceSubmit(topUp.TradeNo))
	require.NoError(t, err)

	assert.Equal(t, user.Id, app.UserId)
	assert.Equal(t, topUp.Id, app.TopUpId)
	assert.Equal(t, topUp.TradeNo, app.TradeNo)
	assert.Equal(t, topUp.Amount, app.Amount)
	assert.Equal(t, topUp.Money, app.Money)
	assert.Equal(t, InvoiceStatusPending, app.Status)
	assert.False(t, app.HasPdf)
	assert.Greater(t, app.CreatedAt, int64(0))
	require.Len(t, app.Orders, 1)
	assert.Equal(t, topUp.TradeNo, app.Orders[0].TradeNo)
	assert.Equal(t, topUp.Amount, app.Orders[0].Amount)
}

func TestSubmitInvoiceApplicationSupportsMultipleOrders(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "multi")
	firstTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_multi_first", common.TopUpStatusSuccess)
	secondTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_multi_second", common.TopUpStatusSuccess)

	input := validInvoiceSubmit("")
	input.TradeNo = ""
	input.TradeNos = []string{firstTopUp.TradeNo, secondTopUp.TradeNo}
	app, err := SubmitInvoiceApplication(user.Id, input)
	require.NoError(t, err)

	assert.Equal(t, firstTopUp.TradeNo, app.TradeNo)
	assert.Equal(t, firstTopUp.Amount+secondTopUp.Amount, app.Amount)
	assert.Equal(t, firstTopUp.Money+secondTopUp.Money, app.Money)
	require.Len(t, app.Orders, 2)
	assert.Equal(t, firstTopUp.TradeNo, app.Orders[0].TradeNo)
	assert.Equal(t, firstTopUp.Amount, app.Orders[0].Amount)
	assert.Equal(t, secondTopUp.TradeNo, app.Orders[1].TradeNo)
	assert.Equal(t, secondTopUp.Amount, app.Orders[1].Amount)

	_, err = SubmitInvoiceApplication(user.Id, validInvoiceSubmit(secondTopUp.TradeNo))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已提交过开票申请")
}

func TestSubmitInvoiceApplicationResubmitsRejectedMultipleOrders(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "multi_resubmit")
	admin := createInvoiceTestUser(t, "multi_resubmit_admin")
	firstTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_multi_resubmit_first", common.TopUpStatusSuccess)
	secondTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_multi_resubmit_second", common.TopUpStatusSuccess)

	input := validInvoiceSubmit("")
	input.TradeNo = ""
	input.TradeNos = []string{firstTopUp.TradeNo, secondTopUp.TradeNo}
	app, err := SubmitInvoiceApplication(user.Id, input)
	require.NoError(t, err)
	app, err = RejectInvoiceApplication(app.Id, admin.Id, "tax id mismatch")
	require.NoError(t, err)

	input.Title = "Updated Multi Technology Co Ltd"
	resubmitted, err := SubmitInvoiceApplication(user.Id, input)
	require.NoError(t, err)

	assert.Equal(t, app.Id, resubmitted.Id)
	assert.Equal(t, InvoiceStatusPending, resubmitted.Status)
	assert.Equal(t, "Updated Multi Technology Co Ltd", resubmitted.Title)
	assert.Equal(t, firstTopUp.Amount+secondTopUp.Amount, resubmitted.Amount)
	require.Len(t, resubmitted.Orders, 2)
}

func TestSubmitInvoiceApplicationRequiresOwnedSuccessfulTopUp(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "owner")
	otherUser := createInvoiceTestUser(t, "other")
	otherTopUp := createInvoiceTestTopUp(t, otherUser.Id, "invoice_other_order", common.TopUpStatusSuccess)
	pendingTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_pending_order", common.TopUpStatusPending)

	_, err := SubmitInvoiceApplication(user.Id, validInvoiceSubmit(otherTopUp.TradeNo))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "充值订单不存在")

	_, err = SubmitInvoiceApplication(user.Id, validInvoiceSubmit(pendingTopUp.TradeNo))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "只有支付成功")
}

func TestSubmitInvoiceApplicationRejectsDuplicateTradeNo(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "duplicate")
	topUp := createInvoiceTestTopUp(t, user.Id, "invoice_duplicate_order", common.TopUpStatusSuccess)

	_, err := SubmitInvoiceApplication(user.Id, validInvoiceSubmit(topUp.TradeNo))
	require.NoError(t, err)

	_, err = SubmitInvoiceApplication(user.Id, validInvoiceSubmit(topUp.TradeNo))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已提交过开票申请")
}

func TestSubmitInvoiceApplicationAllowsRejectedApplicationResubmit(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "resubmit")
	admin := createInvoiceTestUser(t, "resubmit_admin")
	topUp := createInvoiceTestTopUp(t, user.Id, "invoice_resubmit_order", common.TopUpStatusSuccess)

	app, err := SubmitInvoiceApplication(user.Id, validInvoiceSubmit(topUp.TradeNo))
	require.NoError(t, err)
	app, err = RejectInvoiceApplication(app.Id, admin.Id, "title mismatch")
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusRejected, app.Status)

	input := validInvoiceSubmit(topUp.TradeNo)
	input.Title = "Updated Technology Co Ltd"
	input.TaxId = "91350211M000100Y44"
	resubmitted, err := SubmitInvoiceApplication(user.Id, input)
	require.NoError(t, err)

	assert.Equal(t, app.Id, resubmitted.Id)
	assert.Equal(t, InvoiceStatusPending, resubmitted.Status)
	assert.Equal(t, "Updated Technology Co Ltd", resubmitted.Title)
	assert.Equal(t, "91350211M000100Y44", resubmitted.TaxId)
	assert.Empty(t, resubmitted.RejectReason)
	assert.Zero(t, resubmitted.HandlerId)
	assert.Zero(t, resubmitted.HandledAt)
}

func TestGetUserInvoiceTopUpRecordsMarksAppliedOrders(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "records")
	otherUser := createInvoiceTestUser(t, "records_other")
	appliedTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_records_applied", common.TopUpStatusSuccess)
	openTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_records_open", common.TopUpStatusSuccess)
	createInvoiceTestTopUp(t, user.Id, "invoice_records_pending", common.TopUpStatusPending)
	createInvoiceTestTopUp(t, otherUser.Id, "invoice_records_other", common.TopUpStatusSuccess)

	app, err := SubmitInvoiceApplication(user.Id, validInvoiceSubmit(appliedTopUp.TradeNo))
	require.NoError(t, err)

	records, total, err := GetUserInvoiceTopUpRecords(user.Id, &common.PageInfo{Page: 1, PageSize: 10}, "")
	require.NoError(t, err)

	assert.EqualValues(t, 2, total)
	require.Len(t, records, 2)

	byTradeNo := make(map[string]*InvoiceTopUpRecord, len(records))
	for _, record := range records {
		byTradeNo[record.TradeNo] = record
	}

	require.Contains(t, byTradeNo, appliedTopUp.TradeNo)
	assert.True(t, byTradeNo[appliedTopUp.TradeNo].InvoiceApplied)
	assert.Equal(t, app.Id, byTradeNo[appliedTopUp.TradeNo].InvoiceApplicationId)
	assert.Equal(t, InvoiceStatusPending, byTradeNo[appliedTopUp.TradeNo].InvoiceStatus)

	require.Contains(t, byTradeNo, openTopUp.TradeNo)
	assert.False(t, byTradeNo[openTopUp.TradeNo].InvoiceApplied)
}

func TestInvoiceApplicationReviewTransitions(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "review")
	admin := createInvoiceTestUser(t, "admin")
	admin.Role = common.RoleAdminUser
	require.NoError(t, DB.Save(admin).Error)

	approvedTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_review_approved", common.TopUpStatusSuccess)
	approvedApp, err := SubmitInvoiceApplication(user.Id, validInvoiceSubmit(approvedTopUp.TradeNo))
	require.NoError(t, err)

	_, err = ApproveInvoiceApplication(approvedApp.Id, admin.Id, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "请上传发票 PDF 文件")

	approvedApp, err = ApproveInvoiceApplication(approvedApp.Id, admin.Id, "invoice.pdf", "stored.pdf")
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusApproved, approvedApp.Status)
	assert.True(t, approvedApp.HasPdf)
	assert.Equal(t, admin.Id, approvedApp.HandlerId)

	_, err = RejectInvoiceApplication(approvedApp.Id, admin.Id, "wrong state")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "只能处理待审核")

	rejectedTopUp := createInvoiceTestTopUp(t, user.Id, "invoice_review_rejected", common.TopUpStatusSuccess)
	rejectedApp, err := SubmitInvoiceApplication(user.Id, validInvoiceSubmit(rejectedTopUp.TradeNo))
	require.NoError(t, err)

	rejectedApp, err = RejectInvoiceApplication(rejectedApp.Id, admin.Id, "invalid title")
	require.NoError(t, err)
	assert.Equal(t, InvoiceStatusRejected, rejectedApp.Status)
	assert.Equal(t, "invalid title", rejectedApp.RejectReason)

	_, err = ApproveInvoiceApplication(rejectedApp.Id, admin.Id, "invoice.pdf", "stored.pdf")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "只能处理待审核")
}

func TestGetUserInvoiceApplicationByIdRequiresOwner(t *testing.T) {
	truncateTables(t)

	user := createInvoiceTestUser(t, "download_owner")
	otherUser := createInvoiceTestUser(t, "download_other")
	topUp := createInvoiceTestTopUp(t, user.Id, "invoice_download_owner", common.TopUpStatusSuccess)
	app, err := SubmitInvoiceApplication(user.Id, validInvoiceSubmit(topUp.TradeNo))
	require.NoError(t, err)

	owned, err := GetUserInvoiceApplicationById(user.Id, app.Id)
	require.NoError(t, err)
	assert.Equal(t, app.Id, owned.Id)

	_, err = GetUserInvoiceApplicationById(otherUser.Id, app.Id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "开票申请不存在")
}

func TestValidateInvoiceApplicationInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(input *InvoiceApplicationSubmit)
		wantErr string
	}{
		{
			name: "missing title",
			mutate: func(input *InvoiceApplicationSubmit) {
				input.Title = " "
			},
			wantErr: "发票抬头",
		},
		{
			name: "invalid tax id",
			mutate: func(input *InvoiceApplicationSubmit) {
				input.TaxId = "123"
			},
			wantErr: "统一社会信用代码",
		},
		{
			name: "invalid phone",
			mutate: func(input *InvoiceApplicationSubmit) {
				input.BuyerPhone = "abc"
			},
			wantErr: "电话号码",
		},
		{
			name: "valid optional phone omitted",
			mutate: func(input *InvoiceApplicationSubmit) {
				input.BuyerPhone = ""
			},
			wantErr: "",
		},
	}

	for idx, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validInvoiceSubmit(fmt.Sprintf("invoice_validation_%d", idx))
			tc.mutate(&input)

			err := ValidateInvoiceApplicationInput(input)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
