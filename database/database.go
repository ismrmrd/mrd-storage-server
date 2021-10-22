package database

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/ismrmrd/mrd-storage-api/core"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrInvalidContinuationToken = errors.New("invalid continuation token")
	ErrRecordNotFound           = errors.New("record not found")
)

type blobMetadata struct {
	Subject     string               `gorm:"size:64;not null;primaryKey;index:idx_blob_metadata_search,priority:1"`
	Id          uuid.UUID            `gorm:"type:uuid;primaryKey;index:idx_blob_metadata_search,priority:5"`
	Device      sql.NullString       `gorm:"size:64;index:idx_blob_metadata_search,priority:2"`
	Name        sql.NullString       `gorm:"size:64;index:idx_blob_metadata_search,priority:3"`
	Session     sql.NullString       `gorm:"size:64;"`
	ContentType sql.NullString       `gorm:"size:64;"`
	CreatedAt   int64                `gorm:"autoCreateTime:milli;index:idx_blob_metadata_search,priority:4"`
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

func ConnectPostgresqlDatabase(connectionString string) (core.MetadataDatabase, error) {
	dialector := postgres.New(postgres.Config{
		DSN:                  connectionString,
		PreferSimpleProtocol: true,
	})

	return createRepository(dialector)
}

func createRepository(dialector gorm.Dialector) (core.MetadataDatabase, error) {
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Warn),
		SkipDefaultTransaction: true,
	})

	if err != nil {
		return nil, err
	}

	return databaseRepository{db: db}, db.AutoMigrate(&blobMetadata{}, &customBlobMetadata{})
}

func (r databaseRepository) CreateBlobMetadata(id uuid.UUID, tags *core.BlobTags) error {
	metadata := blobMetadata{
		Subject:     tags.Subject,
		Id:          id,
		Device:      toNullString(tags.Device),
		Name:        toNullString(tags.Name),
		Session:     toNullString(tags.Session),
		ContentType: toNullString(tags.ContentType),
	}

	if len(tags.CustomTags) == 0 {
		return r.db.Create(&metadata).Error
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&metadata).Error; err != nil {
			return err
		}

		customMetadata := make([]customBlobMetadata, 0, len(tags.CustomTags))

		for tagName, tagValues := range tags.CustomTags {
			for _, tagValue := range tagValues {
				item := customBlobMetadata{
					BlobSubject: tags.Subject,
					BlobId:      id,
					TagName:     strings.ToLower(tagName),
					TagValue:    tagValue,
				}

				customMetadata = append(customMetadata, item)
			}
		}

		return tx.Create(customMetadata).Error
	})
}

func (r databaseRepository) GetBlobMetadata(subject string, id uuid.UUID) (*core.BlobInfo, error) {
	subquery := r.db.Model(&blobMetadata{}).Where("subject = ? AND id = ?", subject, id)
	blobs, err := r.readTagsFromMetadataSubquery(subquery)

	if err != nil {
		return nil, err
	}

	if len(blobs) == 0 {
		return nil, ErrRecordNotFound
	}

	return &blobs[0], nil
}

func (r databaseRepository) SearchBlobMetadata(tags map[string][]string, at *time.Time, ct *core.ContinutationToken, pageSize int) ([]core.BlobInfo, *core.ContinutationToken, error) {

	subquery := r.db.Model(&blobMetadata{})

	for k, values := range tags {
		switch k {
		case "subject", "device", "name", "session":
			for _, v := range values {
				subquery = subquery.Where(fmt.Sprintf("%s = ?", k), v)
			}

		default:
			for _, v := range values {
				subquery = subquery.Where("EXISTS (SELECT * FROM custom_blob_metadata WHERE blob_id = id AND blob_subject = subject and tag_name = ? AND tag_value = ?)", k, v)
			}
		}
	}

	subquery = subquery.
		Order("created_at DESC, id DESC").
		Limit(pageSize + 1)

	if at != nil {
		subquery = subquery.Where("created_at <= ?", at.UnixMilli())
	}

	if ct != nil {
		c, err := fromContinuationToken(*ct)
		if err != nil {
			return nil, nil, ErrInvalidContinuationToken
		}

		if c.Id == nil {
			subquery = subquery.Where("created_at < ?", c.CreatedTimeMs)
		} else {
			subquery = subquery.Where("created_at = ? AND id < ? OR created_at < ?", c.CreatedTimeMs, c.Id, c.CreatedTimeMs)
		}
	}

	results, err := r.readTagsFromMetadataSubquery(subquery)
	if err != nil {
		return nil, nil, err
	}

	if len(results) > pageSize {

		lastResult, nextResult := results[len(results)-2], results[len(results)-1]

		var c continuation
		if lastResult.CreatedAt == nextResult.CreatedAt {
			// the timestamp is the same between the last entry of this page and the first entry of the next page
			// so we will need to include the ID in the continuation token
			c = continuation{lastResult.CreatedAt.UnixMilli(), &lastResult.Id}
		} else {
			// the common path, where we will be able to generate a simpler WHERE clause
			c = continuation{lastResult.CreatedAt.UnixMilli(), nil}
		}

		ct := toContinuationToken(c)

		return results[:pageSize], &ct, err
	}

	return results, nil, nil
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

func (r databaseRepository) readTagsFromMetadataSubquery(subquery *gorm.DB) ([]core.BlobInfo, error) {

	rows, err := r.db.Table("(?) as md", subquery).
		Select(`md.subject, 
				md.id, 
				md.device, 
				md.name, 
				md.session, 
				md.content_type, 
				md.created_at, 
				custom_blob_metadata.tag_name, 
				custom_blob_metadata.tag_value`).
		Joins(`LEFT JOIN custom_blob_metadata 
				ON custom_blob_metadata.blob_subject = md.subject 
				AND custom_blob_metadata.blob_id = md.id`).
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

		err = rows.Scan(
			&tmpBlobInfo.Tags.Subject,
			&tmpBlobInfo.Id,
			&tmpBlobInfo.Tags.Device,
			&tmpBlobInfo.Tags.Name,
			&tmpBlobInfo.Tags.Session,
			&tmpBlobInfo.Tags.ContentType,
			&timeValueMs,
			&customTagName,
			&customTagValue)

		if err != nil {
			return nil, err
		}

		tmpBlobInfo.CreatedAt = core.UnixTimeMsToTime(timeValueMs)

		if currentBlobInfo == nil || currentBlobInfo.Id != tmpBlobInfo.Id {
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

func toNullString(stringPointer *string) sql.NullString {
	if stringPointer == nil {
		return sql.NullString{}
	}

	return sql.NullString{String: *stringPointer, Valid: true}
}
