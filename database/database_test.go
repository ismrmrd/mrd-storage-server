package database

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/ismrmrd/mrd-storage-server/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorWhenStageBlobMetadataForgotten(t *testing.T) {
	db, err := OpenSqliteDatabase(path.Join(t.TempDir(), "x.db"))
	require.Nil(t, err)
	id, err := uuid.NewV4()
	require.Nil(t, err)
	key := core.BlobKey{Subject: "a", Id: id}

	err = db.DeleteBlobMetadata(context.Background(), key)

	assert.ErrorIs(t, err, core.ErrBlobNotFound)

	err = db.CompleteStagedBlobMetadata(context.Background(), key)
	assert.ErrorIs(t, err, core.ErrStagedRecordNotFound)
}

func TestStagedBlobMetadataCleanedUpOnRevert(t *testing.T) {
	db, err := OpenSqliteDatabase(path.Join(t.TempDir(), "x.db"))
	require.Nil(t, err)
	id, err := uuid.NewV4()
	require.Nil(t, err)
	key := core.BlobKey{Subject: "a", Id: id}

	_, err = db.StageBlobMetadata(context.Background(), key, &core.BlobTags{CustomTags: map[string][]string{"foo": {"bar"}}})
	require.Nil(t, err)

	err = db.DeleteBlobMetadata(context.Background(), key)
	require.Nil(t, err)

	err = db.DeleteBlobMetadata(context.Background(), key)
	assert.ErrorIs(t, err, core.ErrBlobNotFound)

	_, err = db.StageBlobMetadata(context.Background(), key, &core.BlobTags{})
	require.Nil(t, err)
	err = db.CompleteStagedBlobMetadata(context.Background(), key)
	require.Nil(t, err)
	blobInfo, err := db.GetBlobMetadata(context.Background(), key, time.Now())
	require.Nil(t, err)
	assert.Empty(t, blobInfo.Tags.CustomTags, "Residual custom tags remain after DeleteBlobMetadata call")
}

func TestExpiredMetadataIsInvisible(t *testing.T) {
	db, err := OpenSqliteDatabase(path.Join(t.TempDir(), "x.db"))
	require.Nil(t, err)

	id, err := uuid.NewV4()
	require.Nil(t, err)
	key := core.BlobKey{Subject: "blob-expiration-subject", Id: id}

	expiration := "5m"

	_, err = db.StageBlobMetadata(context.Background(), key, &core.BlobTags{TimeToLive: &expiration})
	require.Nil(t, err)

	err = db.CompleteStagedBlobMetadata(context.Background(), key)
	require.Nil(t, err)

	blobInfo, err := db.GetBlobMetadata(context.Background(), key, time.Now())
	require.Nil(t, err)
	require.NotNil(t, blobInfo)

	blobInfo, err = db.GetBlobMetadata(context.Background(), key, time.Now().Add(10*time.Minute))
	require.Nil(t, blobInfo)
	require.True(t, errors.Is(err, core.ErrRecordNotFound))
}

func TestSchemaNotDowngraded(t *testing.T) {
	dbPath := path.Join(t.TempDir(), "x.db")
	_, err := OpenSqliteDatabase(dbPath)
	require.Nil(t, err)

	db, err := sql.Open("sqlite3", dbPath)
	require.Nil(t, err)
	defer db.Close()

	sqlStmt := `INSERT INTO schema_versions (version, status) VALUES(99, 'complete');`
	_, err = db.Exec(sqlStmt)
	require.Nil(t, err)

	_, err = OpenSqliteDatabase(dbPath)
	assert.ErrorIs(t, err, core.ErrExistingDatabaseSchemaNewer)
}

func TestSchemaVersionUpserted(t *testing.T) {
	dbPath := path.Join(t.TempDir(), "x.db")
	_, err := OpenSqliteDatabase(dbPath)
	require.Nil(t, err)

	db, err := sql.Open("sqlite3", dbPath)
	require.Nil(t, err)
	defer db.Close()

	sqlStmt := `UPDATE schema_versions SET status = 'unknown';`
	_, err = db.Exec(sqlStmt)
	require.Nil(t, err)

	_, err = OpenSqliteDatabase(dbPath)
	assert.Nil(t, err)
}
