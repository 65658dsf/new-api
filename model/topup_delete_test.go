package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteTopUpByID(t *testing.T) {
	testCases := []struct {
		name      string
		status    string
		canDelete bool
	}{
		{name: "failed order", status: common.TopUpStatusFailed, canDelete: true},
		{name: "expired order", status: common.TopUpStatusExpired, canDelete: true},
		{name: "pending order", status: common.TopUpStatusPending, canDelete: false},
		{name: "successful order", status: common.TopUpStatusSuccess, canDelete: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			topUp := &TopUp{
				UserId:     1,
				Amount:     100,
				Money:      1,
				TradeNo:    "delete-topup-" + testCase.status,
				CreateTime: time.Now().Unix(),
				Status:     testCase.status,
			}
			require.NoError(t, topUp.Insert())

			err := DeleteTopUpByID(topUp.Id)
			if testCase.canDelete {
				require.NoError(t, err)
				assert.Nil(t, GetTopUpById(topUp.Id))
				return
			}

			require.ErrorIs(t, err, ErrTopUpDeleteNotAllowed)
			assert.NotNil(t, GetTopUpById(topUp.Id))
		})
	}
}

func TestDeleteTopUpByIDReturnsNotFound(t *testing.T) {
	truncateTables(t)

	err := DeleteTopUpByID(999)

	require.ErrorIs(t, err, ErrTopUpNotFound)
}
