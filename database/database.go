package database

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ismrmrd/mrd-storage-server/core"
	gormzerolog "github.com/mpalmer/gorm-zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	schemaVersionInitial        = 1
	schemaVersionAddExpiresAt   = 2
	schemaVersionLatest         = schemaVersionAddExpiresAt
	schemaVersionCompleteStatus = "complete"
)

type schemaVersion struct {
	Version int `gorm:"primaryKey;not null"`
	Status  string
}

type blobMetadata struct {
	Subject     string         `gorm:"size:64;not null;primaryKey;index:idx_blob_metadata_search,priority:1"`
	Id          uuid.UUID      `gorm:"type:uuid;primaryKey;index:idx_blob_metadata_search,priority:5"`
	Device      sql.NullString `gorm:"size:64;index:idx_blob_metadata_search,priority:2"`
	Name        sql.NullString `gorm:"size:64;index:idx_blob_metadata_search,priority:3"`
	Session     sql.NullString `gorm:"size:64;"`
	ContentType sql.NullString `gorm:"size:64;"`
	CreatedAt   int64          `gorm:"autoCreateTime:milli;index:idx_blob_metadata_search,priority:4;index:staged,where:staged = true"`
	ExpiresAt   sql.NullInt64  `gorm:"index:expires,where:expires_at is not null"`
	Staged      bool
	CustomTags  []customBlobMetadata `gorm:"foreignKey:BlobSubject,BlobId;references:Subject,Id;constraint:OnDelete:CASCADE"`
}

type customBlobMetadata struct {
	BlobSubject string    `gorm:"size:64;not null;uniqueindex:idx_custom_blob_metadata_search,priority:1"`
	BlobId      uuid.UUID `gorm:"type:uuid;uniqueindex:idx_custom_blob_metadata_search,priority:2"`
	TagName     string    `gorm:"size:64;uniqueindex:idx_custom_blob_metadata_search,priority:3"`
	TagValue    string    `gorm:"size:64;uniqueindex:idx_custom_blob_metadata_search,priority:4"`
}

type continuation struct {
	CreatedTimeMs int64      `json:"ts"`
	Id            *uuid.UUID `json:"id,omitempty"`
}

type databaseRepository struct {
	db *gorm.DB
}

func OpenSqliteDatabase(dbPath string) (core.MetadataDatabase, error) {

	if err := os.MkdirAll(path.Dir(dbPath), os.ModePerm); err != nil {
		return nil, fmt.Errorf("unable to create directory for database: %v", err)
	}

	return createRepository(sqlite.Open(dbPath))
}

func ConnectPostgresqlDatabase(connectionString, password string) (core.MetadataDatabase, error) {
	if password != "" {
		connectionString = fmt.Sprintf("%s password=%s", connectionString, password)
	}

	dialector := postgres.New(postgres.Config{
		DSN:                  connectionString,
		PreferSimpleProtocol: true,
	})

	return createRepository(dialector)
}

func createRepository(dialector gorm.Dialector) (core.MetadataDatabase, error) {
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 gormzerolog.Logger{},
		SkipDefaultTransaction: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})

	if err != nil {
		return nil, err
	}

	repository := databaseRepository{db: db}

	if db.Migrator().HasTable(&schemaVersion{}) {
		versionInDatabase := schemaVersion{}
		err := db.Where("status = ?", schemaVersionCompleteStatus).Order("version DESC").Limit(1).Find(&versionInDatabase).Error
		if err != nil {
			return nil, err
		}

		if versionInDatabase.Version > schemaVersionLatest {
			return nil, core.ErrExistingDatabaseSchemaNewer
		}

		if versionInDatabase.Version == schemaVersionLatest {
			return repository, nil
		}
	}

	err = db.AutoMigrate(&schemaVersion{}, &blobMetadata{}, &customBlobMetadata{})
	if err != nil {
		return nil, err
	}

	versionInDatabase := schemaVersion{Version: schemaVersionLatest, Status: schemaVersionCompleteStatus}

	// upsert the schema version
	err = db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&versionInDatabase).Error

	return repository, err
}

func (r databaseRepository) StageBlobMetadata(ctx context.Context, key core.BlobKey, tags *core.BlobTags) (*core.BlobInfo, error) {
	metadata := blobMetadata{
		Subject:     key.Subject,
		Id:          key.Id,
		Device:      toNullString(tags.Device),
		Name:        toNullString(tags.Name),
		Session:     toNullString(tags.Session),
		ContentType: toNullString(tags.ContentType),
		ExpiresAt:   toExpiration(tags.TimeToLive),
		Staged:      true,
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&metadata).Error; err != nil {
			return err
		}

		if len(tags.CustomTags) == 0 {
			return nil
		}

		customMetadata := make([]customBlobMetadata, 0, len(tags.CustomTags))

		for tagName, tagValues := range tags.CustomTags {
			for _, tagValue := range tagValues {
				item := customBlobMetadata{
					BlobSubject: key.Subject,
					BlobId:      key.Id,
					TagName:     strings.ToLower(tagName),
					TagValue:    tagValue,
				}

				customMetadata = append(customMetadata, item)
			}
		}

		return tx.Create(customMetadata).Error
	})

	if err != nil {
		return nil, err
	}

	return &core.BlobInfo{
		Key:       key,
		CreatedAt: core.UnixTimeMsToTime(metadata.CreatedAt),
		ExpiresAt: ExpirationToTime(metadata.ExpiresAt),
		Tags:      *tags}, nil
}

func (r databaseRepository) CompleteStagedBlobMetadata(ctx context.Context, key core.BlobKey) error {
	res := r.db.WithContext(ctx).Model(&blobMetadata{}).Where("subject = ? AND id = ? AND staged = ?", key.Subject, key.Id, true).Update("staged", false)

	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return core.ErrStagedRecordNotFound
	}

	return nil
}

func (r databaseRepository) GetBlobMetadata(ctx context.Context, key core.BlobKey, expiresAfter time.Time) (*core.BlobInfo, error) {
	query := r.db.WithContext(ctx).
		Model(&blobMetadata{}).
		Where("subject = ? AND id = ?", key.Subject, key.Id).
		Where("expires_at > ? OR expires_at is null", expiresAfter.UnixMilli())

	blobs, err := r.readTagsFromMetadataSubquery(ctx, query)

	if err != nil {
		return nil, err
	}

	if len(blobs) == 0 {
		return nil, core.ErrRecordNotFound
	}

	return &blobs[0], nil
}

func (r databaseRepository) DeleteBlobMetadata(ctx context.Context, key core.BlobKey) error {
	return r.db.WithContext(ctx).Transaction(
		func(tx *gorm.DB) error {
			res := tx.
				Where("subject = ? AND id = ?", key.Subject, key.Id).
				Delete(&blobMetadata{})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return core.ErrBlobNotFound
			}

			return tx.
				Where("blob_subject = ? AND blob_id = ?", key.Subject, key.Id).
				Delete(&customBlobMetadata{}).Error
		})
}

func (r databaseRepository) SearchBlobMetadata(ctx context.Context, tags map[string][]string, at *time.Time, ct *core.ContinutationToken, pageSize int, expiresAfter time.Time) ([]core.BlobInfo, *core.ContinutationToken, error) {

	query := r.db.WithContext(ctx).Model(&blobMetadata{})

	for k, values := range tags {
		switch k {
		case "subject", "device", "name", "session":
			for _, v := range values {
				query = query.Where(fmt.Sprintf("%s = ?", k), v)
			}

		default:
			for _, v := range values {
				query = query.Where("EXISTS (SELECT * FROM custom_blob_metadata WHERE blob_id = id AND blob_subject = subject and tag_name = ? AND tag_value = ?)", k, v)
			}
		}
	}

	query = query.
		Where("expires_at > ? OR expires_at is null", expiresAfter.UnixMilli()).
		Order("created_at DESC, id DESC").
		Limit(pageSize + 1)

	if at != nil {
		query = query.Where("created_at <= ?", at.UnixMilli())
	}

	if ct != nil {
		c, err := fromContinuationToken(*ct)
		if err != nil {
			return nil, nil, core.ErrInvalidContinuationToken
		}

		if c.Id == nil {
			query = query.Where("created_at < ?", c.CreatedTimeMs)
		} else {
			query = query.Where("created_at = ? AND id < ? OR created_at < ?", c.CreatedTimeMs, c.Id, c.CreatedTimeMs)
		}
	}

	results, err := r.readTagsFromMetadataSubquery(ctx, query)
	if err != nil {
		return nil, nil, err
	}

	if len(results) > pageSize {

		lastResult, nextResult := results[len(results)-2], results[len(results)-1]

		var c continuation
		if lastResult.CreatedAt == nextResult.CreatedAt {
			// the timestamp is the same between the last entry of this page and the first entry of the next page
			// so we will need to include the ID in the continuation token
			c = continuation{lastResult.CreatedAt.UnixMilli(), &lastResult.Key.Id}
		} else {
			// the common path, where we will be able to generate a simpler WHERE clause
			c = continuation{lastResult.CreatedAt.UnixMilli(), nil}
		}

		ct := toContinuationToken(c)

		return results[:pageSize], &ct, err
	}

	return results, nil, nil
}

func (r databaseRepository) GetPageOfExpiredBlobMetadata(ctx context.Context, olderThan time.Time) ([]core.BlobKey, error) {

	rows, err := r.db.
		Model(blobMetadata{}).
		Select(`subject, id`).
		Where(`staged = ? AND created_at < ?`, true, olderThan.UnixMilli()).
		Or(`expires_at < ?`, olderThan.UnixMilli()).
		Limit(200).
		Rows()

	if err != nil {
		return nil, err
	}

	keys := make([]core.BlobKey, 0)

	for rows.Next() {
		key := core.BlobKey{}

		err = rows.Scan(&key.Subject, &key.Id)
		if err != nil {
			return nil, err
		}

		keys = append(keys, key)
	}

	return keys, nil
}

func toContinuationToken(c continuation) core.ContinutationToken {
	bytes, _ := json.Marshal(c)
	return core.ContinutationToken(base64.RawURLEncoding.EncodeToString(bytes))
}

func fromContinuationToken(ct core.ContinutationToken) (continuation, error) {
	bytes, err := base64.RawURLEncoding.DecodeString(string(ct))
	var c continuation
	if err == nil {
		err = json.Unmarshal(bytes, &c)
	}

	return c, err
}

func (r databaseRepository) readTagsFromMetadataSubquery(ctx context.Context, subquery *gorm.DB) ([]core.BlobInfo, error) {

	rows, err := r.db.WithContext(ctx).Table("(?) as md", subquery).
		Select(`md.subject,
				md.id,
				md.device,
				md.name,
				md.session,
				md.content_type,
				md.created_at,
				md.expires_at,
				custom_blob_metadata.tag_name,
				custom_blob_metadata.tag_value`).
		Joins(`LEFT JOIN custom_blob_metadata
				ON custom_blob_metadata.blob_subject = md.subject
				AND custom_blob_metadata.blob_id = md.id`).
		Where(`md.staged = ?`, false).
		Order("md.created_at DESC, md.id DESC").
		Rows()

	if err != nil {
		return nil, err
	}

	results := make([]core.BlobInfo, 0, 1)
	var currentBlobInfo *core.BlobInfo = nil

	for rows.Next() {

		tmpBlobInfo := core.BlobInfo{}

		var customTagName sql.NullString
		var customTagValue sql.NullString

		var timeValueMs int64
		var expirationValueMs sql.NullInt64

		err = rows.Scan(
			&tmpBlobInfo.Key.Subject,
			&tmpBlobInfo.Key.Id,
			&tmpBlobInfo.Tags.Device,
			&tmpBlobInfo.Tags.Name,
			&tmpBlobInfo.Tags.Session,
			&tmpBlobInfo.Tags.ContentType,
			&timeValueMs,
			&expirationValueMs,
			&customTagName,
			&customTagValue)

		if err != nil {
			return nil, err
		}

		tmpBlobInfo.CreatedAt = core.UnixTimeMsToTime(timeValueMs)
		tmpBlobInfo.ExpiresAt = ExpirationToTime(expirationValueMs)

		if currentBlobInfo == nil || currentBlobInfo.Key.Id != tmpBlobInfo.Key.Id {
			results = append(results, tmpBlobInfo)
			currentBlobInfo = &results[len(results)-1]
			currentBlobInfo.Tags.CustomTags = make(map[string][]string)
		}

		if customTagName.Valid && customTagValue.Valid {
			currentBlobInfo.Tags.CustomTags[customTagName.String] = append(currentBlobInfo.Tags.CustomTags[customTagName.String], customTagValue.String)
		}
	}
	return results, nil
}

func (r databaseRepository) HealthCheck(ctx context.Context) error {
	s := r.db.WithContext(ctx).Exec("SELECT NULL from blob_metadata LIMIT 1")
	err := s.Error
	if err != nil {
		log.Ctx(ctx).Error().Msgf("database health check failed: %v", err)
		return errors.New("failed to connect to database")
	}
	return nil
}

func toNullString(stringPointer *string) sql.NullString {
	if stringPointer == nil {
		return sql.NullString{}
	}

	return sql.NullString{String: *stringPointer, Valid: true}
}

func toExpiration(stringPointer *string) sql.NullInt64 {

	if stringPointer == nil {
		return sql.NullInt64{}
	}

	dur, err := time.ParseDuration(*stringPointer)
	if err != nil {
		return sql.NullInt64{}
	}

	return sql.NullInt64{Int64: time.Now().Add(dur).UnixMilli(), Valid: true}
}

func ExpirationToTime(expiration sql.NullInt64) *time.Time {
	if !expiration.Valid {
		return nil
	}

	value := core.UnixTimeMsToTime(expiration.Int64)
	return &value
}
