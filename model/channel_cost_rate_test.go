package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyChannelForCostRateMigration struct {
	Id   int    `gorm:"primaryKey"`
	Key  string `gorm:"not null"`
	Name string
}

func (legacyChannelForCostRateMigration) TableName() string {
	return "channels"
}

func setupChannelCostRateTestDB(t *testing.T) {
	t.Helper()
	previousDB := DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "channel-cost-rate.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	DB = db
	common.MemoryCacheEnabled = true
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}))
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		channelSyncLock.Lock()
		channelsIDM = nil
		group2model2channels = nil
		channel2advancedCustomConfig = nil
		channelSyncLock.Unlock()
	})
}

func TestChannelCostRateDefaultsToOneAndPreservesExplicitZero(t *testing.T) {
	setupChannelCostRateTestDB(t)

	defaultRate := &Channel{Key: "default-rate", Name: "default-rate"}
	require.NoError(t, DB.Create(defaultRate).Error)
	assert.Equal(t, 1.0, defaultRate.CostRate)

	zeroRate := &Channel{Key: "zero-rate", Name: "zero-rate"}
	require.NoError(t, zeroRate.Insert())
	var stored Channel
	require.NoError(t, DB.First(&stored, zeroRate.Id).Error)
	assert.Zero(t, stored.CostRate)
}

func TestBatchInsertChannelsPreservesExplicitZeroCostRate(t *testing.T) {
	setupChannelCostRateTestDB(t)

	channels := []Channel{
		{Key: "zero-rate", Name: "zero-rate", CostRate: 0},
		{Key: "custom-rate", Name: "custom-rate", CostRate: 0.75},
	}
	require.NoError(t, BatchInsertChannels(channels))

	var stored []Channel
	require.NoError(t, DB.Order("name").Find(&stored).Error)
	require.Len(t, stored, 2)
	assert.Equal(t, 0.75, stored[0].CostRate)
	assert.Zero(t, stored[1].CostRate)
}

func TestChannelUpdatePersistsExplicitZeroCostRate(t *testing.T) {
	setupChannelCostRateTestDB(t)

	channel := &Channel{Key: "key", Name: "channel", CostRate: 0.8}
	require.NoError(t, DB.Create(channel).Error)
	channel.CostRate = 0
	require.NoError(t, channel.Update("cost_rate"))

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Zero(t, stored.CostRate)
}

func TestCacheUpdateChannelPreservesCostRate(t *testing.T) {
	setupChannelCostRateTestDB(t)

	channel := &Channel{Id: 123, CostRate: 0.65}
	CacheUpdateChannel(channel)

	cached, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, 0.65, cached.CostRate)
}

func TestChannelCostRateMigrationInitializesExistingRowsToOne(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "channel-cost-rate-migration.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		DB = previousDB
	})

	require.NoError(t, DB.AutoMigrate(&legacyChannelForCostRateMigration{}))
	require.NoError(t, DB.Create(&legacyChannelForCostRateMigration{Key: "legacy", Name: "legacy"}).Error)
	require.NoError(t, DB.AutoMigrate(&Channel{}))

	var migrated Channel
	require.NoError(t, DB.First(&migrated).Error)
	assert.Equal(t, 1.0, migrated.CostRate)
}
