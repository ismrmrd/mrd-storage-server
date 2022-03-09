package core

//go:generate mockgen -destination ../mocks/mocks_core.go -package=mocks github.com/ismrmrd/mrd-storage-server/core MetadataDatabase,BlobStore

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/gofrs/uuid"
)

var (
	ErrInvalidContinuationToken    = errors.New("invalid continuation token")
	ErrRecordNotFound              = errors.New("record not found")
	ErrStagedRecordNotFound        = errors.New("staged metadata not found - was StageBlobMetadata() called beforehand?")
	ErrBlobNotFound                = errors.New("the blob was not found in the store")
	ErrExistingDatabaseSchemaNewer = errors.New("the existing database schema is newer that what the server supports")
)

type BlobKey struct {
	Subject string
	Id      uuid.UUID
}

type BlobTags struct {
	Name        *string
	Device      *string
	Session     *string
	ContentType *string
	TimeToLive  *string
	CustomTags  map[string][]string
}

type BlobInfo struct {
	Key       BlobKey
	Tags      BlobTags
	CreatedAt time.Time
	ExpiresAt *time.Time
}

type ContinutationToken string

func UnixTimeMsToTime(timeValueMs int64) time.Time {
	return time.Unix(timeValueMs/1000, (timeValueMs%1000)*1000000)
}

type MetadataDatabase interface {
	StageBlobMetadata(ctx context.Context, key BlobKey, tags *BlobTags) (*BlobInfo, error)
	CompleteStagedBlobMetadata(ctx context.Context, key BlobKey) error
	DeleteBlobMetadata(ctx context.Context, key BlobKey) error
	GetPageOfExpiredBlobMetadata(ctx context.Context, olderThan time.Time) ([]BlobKey, error)
	GetBlobMetadata(ctx context.Context, key BlobKey, expiresAfter time.Time) (*BlobInfo, error)
	SearchBlobMetadata(ctx context.Context, tags map[string][]string, at *time.Time, ct *ContinutationToken, pageSize int, expiresAfter time.Time) ([]BlobInfo, *ContinutationToken, error)
	HealthCheck(ctx context.Context) error
}

type BlobStore interface {
	SaveBlob(ctx context.Context, contents io.Reader, key BlobKey) error
	ReadBlob(ctx context.Context, writer io.Writer, key BlobKey) error
	DeleteBlob(ctx context.Context, key BlobKey) error
	HealthCheck(ctx context.Context) error
}
