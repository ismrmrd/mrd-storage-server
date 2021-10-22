package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/kelseyhightower/envconfig"
	log "github.com/sirupsen/logrus"

	"github.com/ismrmrd/mrd-storage-api/api"
	"github.com/ismrmrd/mrd-storage-api/core"
	"github.com/ismrmrd/mrd-storage-api/database"
	"github.com/ismrmrd/mrd-storage-api/storage"
)

const (
	ConfigDatabaseProviderSqlite     = "sqlite"
	ConfigDatabaseProviderPostgresql = "postgresql"
	ConfigStorageProviderFileSystem  = "filesystem"
	ConfigStorageProviderAzureBlob   = "azureblob"
)

func main() {
	config := loadConfig()

	handler, err := assembleHandler(config)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.Port), handler))
}

func loadConfig() ConfigSpec {
	var config ConfigSpec
	err := envconfig.Process("MRD_STORAGE_API", &config)
	if err != nil {
		log.Fatal(err.Error())
	}

	return config
}

func assembleHandler(config ConfigSpec) (http.Handler, error) {
	db, err := createMetadataRepository(config)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize metadata database: %v", err)
	}

	blobStore, err := createBlobStore(config)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize storage: %v", err)
	}

	return api.BuildRouter(db, blobStore, config.LogRequests), nil
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

type ConfigSpec struct {
	DatabaseProvider         string `split_words:"true" default:"sqlite"`
	DatabaseConnectionString string `split_words:"true" default:"./_data/metadata.db"`
	StorageProvider          string `split_words:"true" default:"filesystem"`
	StorageConnectionString  string `split_words:"true" default:"./_data/blobs"`
	Port                     int    `split_words:"true" default:"3333"`
	LogRequests              bool   `default:"true"`
}
