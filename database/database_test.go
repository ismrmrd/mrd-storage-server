package database

import (
	"context"
	"database/sql"
	"path"
	"testing"

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
