package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const maxFinancialReportRange = 90 * 24 * time.Hour

func parseFinancialReportRange(c *gin.Context) (int64, int64, bool) {
	now := time.Now()
	start := now.Add(-30 * 24 * time.Hour).Unix()
	end := now.Unix()
	var err error
	if value := c.Query("start_timestamp"); value != "" {
		start, err = strconv.ParseInt(value, 10, 64)
		if err != nil || start <= 0 {
			common.ApiErrorMsg(c, "invalid start_timestamp")
			return 0, 0, false
		}
	}
	if value := c.Query("end_timestamp"); value != "" {
		end, err = strconv.ParseInt(value, 10, 64)
		if err != nil || end <= 0 {
			common.ApiErrorMsg(c, "invalid end_timestamp")
			return 0, 0, false
		}
	}
	if end < start || time.Unix(end, 0).Sub(time.Unix(start, 0)) > maxFinancialReportRange {
		common.ApiErrorMsg(c, "financial report range cannot exceed 90 days")
		return 0, 0, false
	}
	return start, end, true
}

func GetFinancialReport(c *gin.Context) {
	start, end, ok := parseFinancialReportRange(c)
	if !ok {
		return
	}
	channelValue := c.Query("channel_id")
	channelID, err := strconv.Atoi(channelValue)
	if channelValue != "" && (err != nil || channelID <= 0) {
		common.ApiErrorMsg(c, "invalid channel_id")
		return
	}
	rows, err := model.GetChannelFinancialRecords(start, end, channelID, c.Query("model_name"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	historicalRows, err := model.GetHistoricalChannelFinancialRecords(start, end, channelID, c.Query("model_name"), rows)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows = append(rows, historicalRows...)
	report := model.BuildChannelFinancialReport(rows, start, end)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": report})
}
