package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/waffo-com/waffo-go/config"
	"github.com/waffo-com/waffo-go/core"
	"gorm.io/gorm"
)

func setupWaffoStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.TopUp{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestHandleWaffoPaymentOnlyClosesTerminalOrders(t *testing.T) {
	db := setupWaffoStatusTestDB(t)
	webhookHandler := core.NewWebhookHandler(&config.WaffoConfig{})

	tests := []struct {
		name       string
		status     string
		wantStatus string
	}{
		{name: "payment in progress", status: core.OrderStatusPayInProgress, wantStatus: common.TopUpStatusPending},
		{name: "authorization required", status: core.OrderStatusAuthorizationRequired, wantStatus: common.TopUpStatusPending},
		{name: "authorized waiting capture", status: core.OrderStatusAuthedWaitingCapture, wantStatus: common.TopUpStatusPending},
		{name: "unknown future status", status: "FUTURE_STATUS", wantStatus: common.TopUpStatusPending},
		{name: "order closed", status: core.OrderStatusOrderClose, wantStatus: common.TopUpStatusFailed},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tradeNo := fmt.Sprintf("waffo-status-%d", index)
			require.NoError(t, db.Create(&model.TopUp{
				UserId:          1,
				Amount:          10,
				Money:           10,
				TradeNo:         tradeNo,
				PaymentMethod:   model.PaymentMethodWaffo,
				PaymentProvider: model.PaymentProviderWaffo,
				Status:          common.TopUpStatusPending,
			}).Error)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
			handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
				MerchantOrderID: tradeNo,
				OrderStatus:     test.status,
			})

			var topUp model.TopUp
			require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&topUp).Error)
			assert.Equal(t, test.wantStatus, topUp.Status)
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.JSONEq(t, `{"message":"success"}`, recorder.Body.String())
		})
	}
}
