package core

//go:generate mockgen -destination ../mocks/mocks_core.go -package=mocks github.com/ismrmrd/mrd-storage-api/core MetadataDatabase,BlobStore

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/gofrs/uuid"
)

var (
	ErrInvalidContinuationToken = errors.New("invalid continuation token")
	ErrRecordNotFound           = errors.New("record not found")
	ErrStagedRecordNotFound     = errors.New("staged metadata not found - was StageBlobMetadata() called beforehand?")
	ErrBlobNotFound             = errors.New("the blob was not found in the store")
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
	CustomTags  map[string][]string
}

type BlobInfo struct {
	Key       BlobKey
	CreatedAt time.Time
	Tags      BlobTags
}

type ContinutationToken string

func UnixTimeMsToTime(timeValueMs int64) time.Time {
	return time.Unix(timeValueMs/1000, (timeValueMs%1000)*1000000)
}

type MetadataDatabase interface {
	StageBlobMetadata(ctx context.Context, key BlobKey, tags *BlobTags) error
	CompleteStagedBlobMetadata(ctx context.Context, key BlobKey) error
	RevertStagedBlobMetadata(ctx context.Context, key BlobKey) error
	GetPageOfExpiredStagedBlobMetadata(ctx context.Context, olderThan time.Time) ([]BlobKey, error)
	GetBlobMetadata(ctx context.Context, key BlobKey) (*BlobInfo, error)
	SearchBlobMetadata(ctx context.Context, tags map[string][]string, at *time.Time, ct *ContinutationToken, pageSize int) ([]BlobInfo, *ContinutationToken, error)
}

type BlobStore interface {
	SaveBlob(ctx context.Context, contents io.Reader, key BlobKey) error
	ReadBlob(ctx context.Context, writer io.Writer, key BlobKey) error
	DeleteBlob(ctx context.Context, key BlobKey) error
}
