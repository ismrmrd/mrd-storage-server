package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	log "github.com/sirupsen/logrus"

	"github.com/ismrmrd/mrd-storage-server/api"
	"github.com/ismrmrd/mrd-storage-server/core"
	"github.com/ismrmrd/mrd-storage-server/database"
	"github.com/ismrmrd/mrd-storage-server/storage"
)

const (
	ConfigDatabaseProviderSqlite     = "sqlite"
	ConfigDatabaseProviderPostgresql = "postgresql"
	ConfigStorageProviderFileSystem  = "filesystem"
	ConfigStorageProviderAzureBlob   = "azureblob"
)

func main() {
	config := loadConfig()

	db, blobStore, err := assembleDataStores(config)
	if err != nil {
		log.Fatal(err)
	}

	handler := assembleHandler(db, blobStore, config)

	go garbageCollectionLoop(context.Background(), db, blobStore)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), handler))
}

func loadConfig() ConfigSpec {
	var config ConfigSpec
	err := envconfig.Process("MRD_STORAGE_SERVER", &config)
	if err != nil {
		log.Fatal(err.Error())
	}

	return config
}

func assembleDataStores(config ConfigSpec) (core.MetadataDatabase, core.BlobStore, error) {
	db, err := createMetadataRepository(config)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to initialize metadata database: %v", err)
	}

	blobStore, err := createBlobStore(config)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to initialize storage: %v", err)
	}

	return db, blobStore, nil
}

func assembleHandler(db core.MetadataDatabase, blobStore core.BlobStore, config ConfigSpec) http.Handler {
	return api.BuildRouter(db, blobStore, config.LogRequests)
}

func createMetadataRepository(config ConfigSpec) (core.MetadataDatabase, error) {
	switch strings.ToLower(config.DatabaseProvider) {
	case ConfigDatabaseProviderSqlite:
		return database.OpenSqliteDatabase(config.DatabaseConnectionString)
	case ConfigDatabaseProviderPostgresql:
		return database.ConnectPostgresqlDatabase(config.DatabaseConnectionString)
	}

	return nil, fmt.Errorf("unrecognized database provider '%s'", config.DatabaseProvider)
}

func createBlobStore(config ConfigSpec) (core.BlobStore, error) {
	switch strings.ToLower(config.StorageProvider) {
	case ConfigStorageProviderFileSystem:
		return storage.NewFileSystemStore(config.StorageConnectionString)
	case ConfigStorageProviderAzureBlob:
		return storage.NewAzureBlobStore(config.StorageConnectionString)
	}
	return nil, fmt.Errorf("unrecognized storage provider '%s'", config.StorageProvider)
}

func garbageCollectionLoop(ctx context.Context, db core.MetadataDatabase, blobStore core.BlobStore) {
	ticker := time.NewTicker(30 * time.Minute)
	for range ticker.C {
		for i := 0; i < 10; i++ {
			log.Info("Begining garbage collection")
			err := core.CollectGarbage(ctx, db, blobStore, time.Now().Add(-30*time.Minute).UTC())
			if err == nil {
				log.Info("Garbage collection completed")
				break
			}

			log.Errorf("Garbage collection failed: %v", err)
			time.Sleep(30 * time.Second)
		}
	}
}

type ConfigSpec struct {
	DatabaseProvider         string `split_words:"true" default:"sqlite"`
	DatabaseConnectionString string `split_words:"true" default:"_data/metadata.db"`
	StorageProvider          string `split_words:"true" default:"filesystem"`
	StorageConnectionString  string `split_words:"true" default:"_data/blobs"`
	Port                     int    `split_words:"true" default:"3333"`
	LogRequests              bool   `split_words:"true" default:"true"`
}
