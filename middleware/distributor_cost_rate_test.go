package middleware

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSetupContextForSelectedChannelKeepsRequestStartRateAcrossRetries(t *testing.T) {
	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "distributor-cost-rate.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelCostRateVersion{}))
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
		require.NoError(t, sqlDB.Close())
	})

	channel := &model.Channel{Key: "test-key", Name: "channel", CostRate: 1.2}
	require.NoError(t, db.Create(channel).Error)
	initialVersion := &model.ChannelCostRateVersion{ChannelId: channel.Id, CostRate: 0.8, EffectiveAt: 0}
	require.NoError(t, db.Create(initialVersion).Error)
	require.NoError(t, db.Create(&model.ChannelCostRateVersion{
		ChannelId:   channel.Id,
		CostRate:    1.2,
		EffectiveAt: time.Unix(200, 0).UnixNano(),
	}).Error)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Unix(150, 0))
	require.Nil(t, SetupContextForSelectedChannel(ctx, channel, "gpt-test"))

	rate, ok := common.GetContextKeyType[float64](ctx, constant.ContextKeyChannelCostRate)
	require.True(t, ok)
	assert.Equal(t, 0.8, rate)
	versionID, ok := common.GetContextKeyType[int64](ctx, constant.ContextKeyChannelCostRateVersionId)
	require.True(t, ok)
	assert.Equal(t, initialVersion.Id, versionID)

	require.NoError(t, db.Model(channel).Update("cost_rate", 0.5).Error)
	require.NoError(t, db.Create(&model.ChannelCostRateVersion{
		ChannelId:   channel.Id,
		CostRate:    0.5,
		EffectiveAt: time.Unix(300, 0).UnixNano(),
	}).Error)
	require.Nil(t, SetupContextForSelectedChannel(ctx, channel, "gpt-test"))

	rate, ok = common.GetContextKeyType[float64](ctx, constant.ContextKeyChannelCostRate)
	require.True(t, ok)
	assert.Equal(t, 0.8, rate)
	versionID, ok = common.GetContextKeyType[int64](ctx, constant.ContextKeyChannelCostRateVersionId)
	require.True(t, ok)
	assert.Equal(t, initialVersion.Id, versionID)
}
