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
		{name: "successful order", status: common.TopUpStatusSuccess, canDelete: true},
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

func TestDeleteSuccessfulTopUpByIDRetainsCreditedRewards(t *testing.T) {
	truncateTables(t)

	creditedUser := &User{
		Id:       101,
		Username: "topup_delete_user",
		Status:   common.UserStatusEnabled,
		Quota:    12345,
		AffCode:  "topup-delete-user",
	}
	inviter := &User{
		Id:              102,
		Username:        "topup_delete_inviter",
		Status:          common.UserStatusEnabled,
		AffCode:         "topup-delete-inviter",
		AffQuota:        678,
		AffHistoryQuota: 910,
	}
	require.NoError(t, DB.Create(creditedUser).Error)
	require.NoError(t, DB.Create(inviter).Error)

	topUp := &TopUp{
		UserId:     creditedUser.Id,
		Amount:     100,
		Money:      1,
		TradeNo:    "delete-successful-topup",
		CreateTime: time.Now().Unix(),
		Status:     common.TopUpStatusSuccess,
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, DeleteTopUpByID(topUp.Id))

	var creditedUserAfter User
	require.NoError(t, DB.First(&creditedUserAfter, creditedUser.Id).Error)
	assert.Equal(t, creditedUser.Quota, creditedUserAfter.Quota)

	var inviterAfter User
	require.NoError(t, DB.First(&inviterAfter, inviter.Id).Error)
	assert.Equal(t, inviter.AffQuota, inviterAfter.AffQuota)
	assert.Equal(t, inviter.AffHistoryQuota, inviterAfter.AffHistoryQuota)
	assert.Nil(t, GetTopUpById(topUp.Id))
}

func TestDeleteTopUpByIDReturnsNotFound(t *testing.T) {
	truncateTables(t)

	err := DeleteTopUpByID(999)

	require.ErrorIs(t, err, ErrTopUpNotFound)
}
