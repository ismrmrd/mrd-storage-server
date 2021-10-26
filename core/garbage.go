package core

import (
	"context"
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
)

// When we write a blob, we first stage it in the metadata database, then write it to the blob store, then
// promote the staged metadata to completed. If the write fails to the blob store, we delete the staged metadata. But
// if the process crashes after the blob write but before we can complete the metadata, there will be orphaned blobs
// in the blob store. This function deletes these orphaned blobs from the blob store and their corresponding staged
// records in the medatada database.
func CollectGarbage(ctx context.Context, db MetadataDatabase, store BlobStore, olderThan time.Time) error {
	for {
		expiredKeys, err := db.GetPageOfExpiredStagedBlobMetadata(ctx, olderThan)
		if err != nil {
			return err
		}

		if len(expiredKeys) == 0 {
			return nil
		}

		for _, key := range expiredKeys {
			err = processExpiredKey(ctx, db, store, key)
			if err != nil {
				return err
			}
		}
	}
}

func processExpiredKey(ctx context.Context, db MetadataDatabase, store BlobStore, key BlobKey) error {
	log.Infof("Removing expired key %v", key)
	if err := store.DeleteBlob(ctx, key); err != nil {
		return err
	}

	if err := db.RevertStagedBlobMetadata(ctx, key); err != nil && !errors.Is(err, ErrStagedRecordNotFound) {
		return err
	}

	return nil
}
