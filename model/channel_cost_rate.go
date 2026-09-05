package model

import (
	"errors"
	"math"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelCostRateVersion is the immutable, time-versioned source for channel
// financial cost rates. EffectiveAt is a Unix nanosecond timestamp so a rate
// adjustment never shares an effective boundary with another adjustment.
type ChannelCostRateVersion struct {
	Id          int64   `json:"id" gorm:"primaryKey"`
	ChannelId   int     `json:"channel_id" gorm:"not null;uniqueIndex:idx_channel_cost_rate_effective,priority:1"`
	CostRate    float64 `json:"cost_rate" gorm:"not null"`
	EffectiveAt int64   `json:"effective_at" gorm:"not null;uniqueIndex:idx_channel_cost_rate_effective,priority:2"`
}

// ChannelCostRateSnapshot is frozen when a channel is selected for a request.
// It deliberately carries the version identity alongside the rate so later
// settlement and asynchronous task adjustments never need to reinterpret the
// request against a newer channel configuration.
type ChannelCostRateSnapshot struct {
	VersionId   int64
	CostRate    float64
	EffectiveAt int64
}

func (v *ChannelCostRateVersion) BeforeCreate(tx *gorm.DB) error {
	if v.ChannelId <= 0 {
		return errors.New("channel cost rate version requires a channel id")
	}
	if !isValidChannelCostRate(v.CostRate) {
		return errors.New("channel cost rate must be a non-negative finite number")
	}
	if v.EffectiveAt < 0 {
		return errors.New("channel cost rate effective time must not be negative")
	}
	return nil
}

func isValidChannelCostRate(rate float64) bool {
	return rate >= 0 && !math.IsNaN(rate) && !math.IsInf(rate, 0)
}

// IsValidChannelCostRate reports whether a configured rate is finite and
// non-negative. There is deliberately no upper bound on this multiplier.
func IsValidChannelCostRate(rate float64) bool {
	return isValidChannelCostRate(rate)
}

// ValidateChannelCostRate validates an administrator-configured cost rate.
// The rate is a multiplier and intentionally has no upper bound; the only
// invalid values are negative, NaN, and infinities. Zero remains valid for
// backwards compatibility with channels configured without upstream cost.
func ValidateChannelCostRate(rate float64) error {
	if !isValidChannelCostRate(rate) {
		return errors.New("channel cost rate must be a non-negative finite number")
	}
	return nil
}

func normalizedChannelCostRate(rate float64) float64 {
	if !isValidChannelCostRate(rate) {
		return 1
	}
	return rate
}

func channelCostRateEffectiveAt(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func channelCostRateEffectiveAtFromUnixSeconds(timestamp int64) int64 {
	if timestamp <= 0 {
		return 0
	}
	return time.Unix(timestamp, 0).UnixNano()
}

// InitializeChannelCostRateVersions creates an unbounded-past baseline for
// every channel that predates this feature. Existing installations cannot
// reconstruct older edits, so the current persisted rate remains the same
// fallback that historical financial reports used before versioning.
func InitializeChannelCostRateVersions() error {
	if DB == nil {
		return errors.New("main database is not initialized")
	}
	if !DB.Migrator().HasTable(&Channel{}) || !DB.Migrator().HasTable(&ChannelCostRateVersion{}) {
		return nil
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := tx.Select("id", "cost_rate").Find(&channels).Error; err != nil {
			return err
		}
		if len(channels) == 0 {
			return nil
		}

		var existing []ChannelCostRateVersion
		if err := tx.Select("channel_id").Find(&existing).Error; err != nil {
			return err
		}
		hasVersion := make(map[int]struct{}, len(existing))
		for _, version := range existing {
			hasVersion[version.ChannelId] = struct{}{}
		}

		versions := make([]ChannelCostRateVersion, 0, len(channels))
		for _, channel := range channels {
			if _, exists := hasVersion[channel.Id]; exists {
				continue
			}
			if !isValidChannelCostRate(channel.CostRate) {
				return errors.New("existing channel cost rate must be a non-negative finite number")
			}
			versions = append(versions, ChannelCostRateVersion{
				ChannelId:   channel.Id,
				CostRate:    channel.CostRate,
				EffectiveAt: 0,
			})
		}
		if len(versions) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&versions).Error
	})
	return err
}

// GetChannelCostRateAt resolves the rate that was effective when a request
// began. A database failure never changes user billing: callers receive their
// current channel snapshot as the fallback and can record the error for
// administrator diagnostics.
func GetChannelCostRateAt(channelID int, requestAt time.Time, fallback float64) (float64, error) {
	snapshot, err := GetChannelCostRateSnapshotAt(channelID, requestAt, fallback)
	return snapshot.CostRate, err
}

func GetChannelCostRateSnapshotAt(channelID int, requestAt time.Time, fallback float64) (ChannelCostRateSnapshot, error) {
	fallback = normalizedChannelCostRate(fallback)
	if channelID <= 0 || requestAt.IsZero() {
		return ChannelCostRateSnapshot{CostRate: fallback}, nil
	}
	return getChannelCostRateSnapshotAtEffectiveTime(channelID, channelCostRateEffectiveAt(requestAt), fallback)
}

// GetCurrentChannelCostRateSnapshot resolves the newest version visible to the
// database at channel-selection time. This intentionally does not use a local
// cache: a configuration change committed on another application node must be
// observed before that node creates a financial snapshot for a new request.
func GetCurrentChannelCostRateSnapshot(channelID int, fallback float64) (ChannelCostRateSnapshot, error) {
	fallback = normalizedChannelCostRate(fallback)
	if channelID <= 0 {
		return ChannelCostRateSnapshot{CostRate: fallback}, nil
	}
	if DB == nil {
		return ChannelCostRateSnapshot{CostRate: fallback}, errors.New("main database is not initialized")
	}
	if !DB.Migrator().HasTable(&ChannelCostRateVersion{}) {
		return ChannelCostRateSnapshot{CostRate: fallback}, nil
	}

	var version ChannelCostRateVersion
	err := DB.Where("channel_id = ?", channelID).
		Order("effective_at DESC").
		Order("id DESC").
		First(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ChannelCostRateSnapshot{CostRate: fallback}, nil
	}
	if err != nil {
		return ChannelCostRateSnapshot{CostRate: fallback}, err
	}
	return channelCostRateSnapshotFromVersion(version, fallback)
}

func getChannelCostRateSnapshotAtEffectiveTime(channelID int, effectiveAt int64, fallback float64) (ChannelCostRateSnapshot, error) {
	if effectiveAt < 0 {
		return ChannelCostRateSnapshot{CostRate: fallback}, errors.New("channel cost rate request time must not be negative")
	}
	if DB == nil {
		return ChannelCostRateSnapshot{CostRate: fallback}, errors.New("main database is not initialized")
	}
	if !DB.Migrator().HasTable(&ChannelCostRateVersion{}) {
		return ChannelCostRateSnapshot{CostRate: fallback}, nil
	}

	var version ChannelCostRateVersion
	err := DB.Where("channel_id = ? AND effective_at <= ?", channelID, effectiveAt).
		Order("effective_at DESC").
		Order("id DESC").
		First(&version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ChannelCostRateSnapshot{CostRate: fallback}, nil
	}
	if err != nil {
		return ChannelCostRateSnapshot{CostRate: fallback}, err
	}
	return channelCostRateSnapshotFromVersion(version, fallback)
}

func channelCostRateSnapshotFromVersion(version ChannelCostRateVersion, fallback float64) (ChannelCostRateSnapshot, error) {
	if !isValidChannelCostRate(version.CostRate) {
		return ChannelCostRateSnapshot{CostRate: fallback}, errors.New("stored channel cost rate version is invalid")
	}
	return ChannelCostRateSnapshot{
		VersionId:   version.Id,
		CostRate:    version.CostRate,
		EffectiveAt: version.EffectiveAt,
	}, nil
}

func getChannelCostRateVersions(channelIDs []int) (map[int][]ChannelCostRateVersion, error) {
	versionsByChannel := make(map[int][]ChannelCostRateVersion)
	if len(channelIDs) == 0 {
		return versionsByChannel, nil
	}
	if DB == nil {
		return nil, errors.New("main database is not initialized")
	}

	var versions []ChannelCostRateVersion
	err := DB.Where("channel_id IN ?", channelIDs).
		Order("channel_id ASC").
		Order("effective_at ASC").
		Order("id ASC").
		Find(&versions).Error
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		versionsByChannel[version.ChannelId] = append(versionsByChannel[version.ChannelId], version)
	}
	return versionsByChannel, nil
}

func resolveChannelCostRateVersion(versions []ChannelCostRateVersion, effectiveAt int64) (ChannelCostRateSnapshot, bool) {
	index := sort.Search(len(versions), func(i int) bool {
		return versions[i].EffectiveAt > effectiveAt
	})
	if index == 0 {
		return ChannelCostRateSnapshot{}, false
	}
	version := versions[index-1]
	if !isValidChannelCostRate(version.CostRate) {
		return ChannelCostRateSnapshot{}, false
	}
	return ChannelCostRateSnapshot{
		VersionId:   version.Id,
		CostRate:    version.CostRate,
		EffectiveAt: version.EffectiveAt,
	}, true
}

// appendChannelCostRateVersion serializes successive changes for one channel.
// Callers must already hold the channel row lock when this accompanies a
// channel update. A timestamp collision is made strictly monotonic so there is
// always one unambiguous rate at every request-start instant.
func appendChannelCostRateVersion(tx *gorm.DB, channelID int, costRate float64, effectiveAt int64) (*ChannelCostRateVersion, error) {
	if tx == nil {
		return nil, errors.New("channel cost rate version transaction is required")
	}
	if channelID <= 0 {
		return nil, errors.New("channel cost rate version requires a channel id")
	}
	if !isValidChannelCostRate(costRate) {
		return nil, errors.New("channel cost rate must be a non-negative finite number")
	}
	if effectiveAt < 0 {
		return nil, errors.New("channel cost rate effective time must not be negative")
	}
	if !tx.Migrator().HasTable(&ChannelCostRateVersion{}) {
		return nil, nil
	}

	var latest ChannelCostRateVersion
	err := lockForUpdate(tx).Where("channel_id = ?", channelID).
		Order("effective_at DESC").
		Order("id DESC").
		First(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		if latest.CostRate == costRate {
			return &latest, nil
		}
		if effectiveAt <= latest.EffectiveAt {
			if latest.EffectiveAt == int64(^uint64(0)>>1) {
				return nil, errors.New("channel cost rate effective time overflow")
			}
			effectiveAt = latest.EffectiveAt + 1
		}
	}

	version := &ChannelCostRateVersion{
		ChannelId:   channelID,
		CostRate:    costRate,
		EffectiveAt: effectiveAt,
	}
	if err := tx.Create(version).Error; err != nil {
		return nil, err
	}
	return version, nil
}

func ensureChannelCostRateVersionBaseline(tx *gorm.DB, channel *Channel) error {
	if channel == nil {
		return errors.New("channel cannot be nil")
	}
	var count int64
	if err := tx.Model(&ChannelCostRateVersion{}).Where("channel_id = ?", channel.Id).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	_, err := appendChannelCostRateVersion(tx, channel.Id, channel.CostRate, 0)
	return err
}
