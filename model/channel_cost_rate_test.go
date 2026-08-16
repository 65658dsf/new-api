package model

import (
	"math"
	"path/filepath"
	"testing"
	"time"

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
	previousDatabaseType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "channel-cost-rate.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = true
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}, &ChannelCostRateVersion{}))
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
		DB = previousDB
		common.SetMainDatabaseType(previousDatabaseType)
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
	var versions []ChannelCostRateVersion
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("effective_at ASC").Find(&versions).Error)
	require.Len(t, versions, 2)
	assert.Equal(t, 0.8, versions[0].CostRate)
	assert.Zero(t, versions[1].CostRate)
	assert.Greater(t, versions[1].EffectiveAt, versions[0].EffectiveAt)
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
	require.NoError(t, DB.AutoMigrate(&ChannelCostRateVersion{}))
	require.NoError(t, InitializeChannelCostRateVersions())
	require.NoError(t, InitializeChannelCostRateVersions())

	var migrated Channel
	require.NoError(t, DB.First(&migrated).Error)
	assert.Equal(t, 1.0, migrated.CostRate)
	var version ChannelCostRateVersion
	require.NoError(t, DB.Where("channel_id = ?", migrated.Id).First(&version).Error)
	assert.Equal(t, 1.0, version.CostRate)
	assert.Zero(t, version.EffectiveAt)
	var versionCount int64
	require.NoError(t, DB.Model(&ChannelCostRateVersion{}).Where("channel_id = ?", migrated.Id).Count(&versionCount).Error)
	assert.Equal(t, int64(1), versionCount)
}

func TestDefaultChannelCostRateChangePreservesEarlierRequests(t *testing.T) {
	setupChannelCostRateTestDB(t)
	channel := &Channel{Key: "key", Name: "default", CostRate: 1}
	require.NoError(t, DB.Create(channel).Error)
	_, err := appendChannelCostRateVersion(DB, channel.Id, 1, 0)
	require.NoError(t, err)
	changeAt := channelCostRateEffectiveAt(time.Unix(200, 0))
	_, err = appendChannelCostRateVersion(DB, channel.Id, 0.75, changeAt)
	require.NoError(t, err)

	before, err := GetChannelCostRateAt(channel.Id, time.Unix(199, 0), channel.CostRate)
	require.NoError(t, err)
	after, err := GetChannelCostRateAt(channel.Id, time.Unix(200, 0), channel.CostRate)
	require.NoError(t, err)

	assert.Equal(t, 1.0, before)
	assert.Equal(t, 0.75, after)
}

func TestCustomChannelCostRateSwitchesAtEffectiveTime(t *testing.T) {
	setupChannelCostRateTestDB(t)
	channel := &Channel{Key: "key", Name: "custom", CostRate: 0.8}
	require.NoError(t, DB.Create(channel).Error)
	_, err := appendChannelCostRateVersion(DB, channel.Id, 0.8, 0)
	require.NoError(t, err)
	changeAt := channelCostRateEffectiveAt(time.Unix(200, 0))
	_, err = appendChannelCostRateVersion(DB, channel.Id, 0.55, changeAt)
	require.NoError(t, err)

	oldRate, err := GetChannelCostRateAt(channel.Id, time.Unix(150, 0), channel.CostRate)
	require.NoError(t, err)
	newRate, err := GetChannelCostRateAt(channel.Id, time.Unix(250, 0), channel.CostRate)
	require.NoError(t, err)

	assert.Equal(t, 0.8, oldRate)
	assert.Equal(t, 0.55, newRate)
}

func TestCurrentChannelCostRateSnapshotUsesLatestCommittedVersion(t *testing.T) {
	setupChannelCostRateTestDB(t)
	channel := &Channel{Key: "key", Name: "current", CostRate: 0.8}
	require.NoError(t, DB.Create(channel).Error)
	initial, err := appendChannelCostRateVersion(DB, channel.Id, 0.8, 0)
	require.NoError(t, err)

	firstSnapshot, err := GetCurrentChannelCostRateSnapshot(channel.Id, channel.CostRate)
	require.NoError(t, err)
	assert.Equal(t, initial.Id, firstSnapshot.VersionId)
	assert.Equal(t, 0.8, firstSnapshot.CostRate)

	updated, err := appendChannelCostRateVersion(DB, channel.Id, 0.55, channelCostRateEffectiveAt(time.Unix(200, 0)))
	require.NoError(t, err)
	secondSnapshot, err := GetCurrentChannelCostRateSnapshot(channel.Id, channel.CostRate)
	require.NoError(t, err)
	assert.Equal(t, updated.Id, secondSnapshot.VersionId)
	assert.Equal(t, 0.55, secondSnapshot.CostRate)
}

func TestChannelCostRateResolvesAcrossMultipleHistoricalVersions(t *testing.T) {
	setupChannelCostRateTestDB(t)
	channel := &Channel{Key: "key", Name: "multiple", CostRate: 0.8}
	require.NoError(t, DB.Create(channel).Error)
	for _, version := range []struct {
		rate        float64
		effectiveAt int64
	}{
		{rate: 0.8, effectiveAt: 0},
		{rate: 0.6, effectiveAt: channelCostRateEffectiveAt(time.Unix(200, 0))},
		{rate: 1.2, effectiveAt: channelCostRateEffectiveAt(time.Unix(300, 0))},
	} {
		_, err := appendChannelCostRateVersion(DB, channel.Id, version.rate, version.effectiveAt)
		require.NoError(t, err)
	}

	for _, test := range []struct {
		requestAt time.Time
		expected  float64
	}{
		{requestAt: time.Unix(100, 0), expected: 0.8},
		{requestAt: time.Unix(250, 0), expected: 0.6},
		{requestAt: time.Unix(350, 0), expected: 1.2},
	} {
		rate, err := GetChannelCostRateAt(channel.Id, test.requestAt, channel.CostRate)
		require.NoError(t, err)
		assert.Equal(t, test.expected, rate)
	}
}

func TestChannelCostRateVersionTimestampCollisionIsMonotonic(t *testing.T) {
	setupChannelCostRateTestDB(t)
	channel := &Channel{Key: "key", Name: "collision", CostRate: 1}
	require.NoError(t, DB.Create(channel).Error)
	_, err := appendChannelCostRateVersion(DB, channel.Id, 1, 0)
	require.NoError(t, err)
	effectiveAt := channelCostRateEffectiveAt(time.Unix(200, 0))
	first, err := appendChannelCostRateVersion(DB, channel.Id, 0.8, effectiveAt)
	require.NoError(t, err)
	second, err := appendChannelCostRateVersion(DB, channel.Id, 0.6, effectiveAt)
	require.NoError(t, err)

	assert.Equal(t, effectiveAt, first.EffectiveAt)
	assert.Equal(t, effectiveAt+1, second.EffectiveAt)
	firstSnapshot, err := getChannelCostRateSnapshotAtEffectiveTime(channel.Id, effectiveAt, channel.CostRate)
	require.NoError(t, err)
	secondSnapshot, err := getChannelCostRateSnapshotAtEffectiveTime(channel.Id, effectiveAt+1, channel.CostRate)
	require.NoError(t, err)
	assert.Equal(t, 0.8, firstSnapshot.CostRate)
	assert.Equal(t, 0.6, secondSnapshot.CostRate)
}

func TestChannelCostRateVersionRejectsInvalidData(t *testing.T) {
	setupChannelCostRateTestDB(t)
	for _, test := range []struct {
		name        string
		rate        float64
		effectiveAt int64
	}{
		{name: "negative rate", rate: -0.1},
		{name: "nan rate", rate: math.NaN()},
		{name: "infinite rate", rate: math.Inf(1)},
		{name: "negative effective time", rate: 1, effectiveAt: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			version := &ChannelCostRateVersion{ChannelId: 1, CostRate: test.rate, EffectiveAt: test.effectiveAt}
			require.Error(t, DB.Create(version).Error)
		})
	}
}
