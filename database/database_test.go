package database

import (
	"context"
	"path"
	"testing"

	"github.com/gofrs/uuid"
	"github.com/ismrmrd/mrd-storage-api/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorWhenStageBlobMetadataForgotten(t *testing.T) {
	db, err := OpenSqliteDatabase(path.Join(t.TempDir(), "x.db"))
	require.Nil(t, err)
	id, err := uuid.NewV4()
	require.Nil(t, err)
	key := core.BlobKey{Subject: "a", Id: id}

	err = db.RevertStagedBlobMetadata(context.Background(), key)

	assert.ErrorIs(t, err, core.ErrStagedRecordNotFound)

	err = db.CompleteStagedBlobMetadata(context.Background(), key)
	assert.ErrorIs(t, err, core.ErrStagedRecordNotFound)
}

func TestStagedBlobMetadataCleanedUpOnCompletion(t *testing.T) {
	db, err := OpenSqliteDatabase(path.Join(t.TempDir(), "x.db"))
	require.Nil(t, err)
	id, err := uuid.NewV4()
	require.Nil(t, err)
	key := core.BlobKey{Subject: "a", Id: id}

	err = db.StageBlobMetadata(context.Background(), key, &core.BlobTags{})
	require.Nil(t, err)

	err = db.CompleteStagedBlobMetadata(context.Background(), key)
	require.Nil(t, err)

	err = db.RevertStagedBlobMetadata(context.Background(), key)
	assert.ErrorIs(t, err, core.ErrStagedRecordNotFound)
}

func TestStagedBlobMetadataCleanedUpOnRevert(t *testing.T) {
	db, err := OpenSqliteDatabase(path.Join(t.TempDir(), "x.db"))
	require.Nil(t, err)
	id, err := uuid.NewV4()
	require.Nil(t, err)
	key := core.BlobKey{Subject: "a", Id: id}

	err = db.StageBlobMetadata(context.Background(), key, &core.BlobTags{})
	require.Nil(t, err)

	err = db.RevertStagedBlobMetadata(context.Background(), key)
	require.Nil(t, err)

	err = db.RevertStagedBlobMetadata(context.Background(), key)
	assert.ErrorIs(t, err, core.ErrStagedRecordNotFound)
}
