package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/ismrmrd/mrd-storage-server/core"
	"github.com/ismrmrd/mrd-storage-server/mocks"
	"github.com/stretchr/testify/assert"
)

// Ensure garbage collection completes even when DeleteBlobMetadata
// returns ErrBlobNotFound, which suggests that another instance
// is performing garbage collection at the same time.
func TestConcurrentGarbageCollection(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	db := mocks.NewMockMetadataDatabase(mockCtrl)
	store := mocks.NewMockBlobStore(mockCtrl)

	key := core.BlobKey{Subject: "s", Id: uuid.UUID{}}

	db.EXPECT().
		GetPageOfExpiredBlobMetadata(gomock.Any(), gomock.Any()).
		Return([]core.BlobKey{key}, nil)

	db.EXPECT().DeleteBlobMetadata(gomock.Any(), key).Return(core.ErrBlobNotFound)

	db.EXPECT().
		GetPageOfExpiredBlobMetadata(gomock.Any(), gomock.Any()).
		Return([]core.BlobKey{}, nil)

	store.EXPECT().DeleteBlob(gomock.Any(), key)

	err := core.CollectGarbage(context.Background(), db, store, time.Now())
	assert.Nil(t, err)
}
