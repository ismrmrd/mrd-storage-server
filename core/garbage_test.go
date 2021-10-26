package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/mock/gomock"
	"github.com/ismrmrd/mrd-storage-api/core"
	"github.com/ismrmrd/mrd-storage-api/mocks"
	"github.com/stretchr/testify/assert"
)

// Ensure garbage collection completes even when RevertStagedBlobMetadata
// returns ErrStagedRecordNotFound, which suggests that another instance
// is performing garbage collection at the same time.
func TestConcurrentGarbageCollection(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	db := mocks.NewMockMetadataDatabase(mockCtrl)
	store := mocks.NewMockBlobStore(mockCtrl)

	key := core.BlobKey{Subject: "s", Id: uuid.UUID{}}

	db.EXPECT().
		GetPageOfExpiredStagedBlobMetadata(gomock.Any(), gomock.Any()).
		Return([]core.BlobKey{key}, nil)

	db.EXPECT().RevertStagedBlobMetadata(gomock.Any(), key).Return(core.ErrStagedRecordNotFound)

	db.EXPECT().
		GetPageOfExpiredStagedBlobMetadata(gomock.Any(), gomock.Any()).
		Return([]core.BlobKey{}, nil)

	store.EXPECT().DeleteBlob(gomock.Any(), key)

	err := core.CollectGarbage(context.Background(), db, store, time.Now())
	assert.Nil(t, err)
}
