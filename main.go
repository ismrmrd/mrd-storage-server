package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/johnstairs/pathenvconfig"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

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

type Args struct {
	PrettyPrint bool   `help:"Pretty-print logs." short:"p"`
	LogLevel    string `help:"Set the minimum log level to emit." short:"l" default:"Info" enum:"Debug,Info,Warn,Error,Fatal,Panic,Disabled"`
}

func main() {
	args := Args{}
	kong.Parse(&args, kong.UsageOnError())

	configureZerolog(args)

	config := loadConfig()

	db, blobStore, err := assembleDataStores(config)
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	handler := assembleHandler(db, blobStore, config)

	go garbageCollectionLoop(context.Background(), db, blobStore)
	err = http.ListenAndServe(fmt.Sprintf(":%d", config.Port), handler)
	log.Fatal().Err(err).Send()
}

func loadConfig() ConfigSpec {
	var config ConfigSpec
	err := pathenvconfig.Process("MRD_STORAGE_SERVER", &config)
	if err != nil {
		log.Fatal().Err(err).Send()
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
		return database.ConnectPostgresqlDatabase(config.DatabaseConnectionString, config.DatabasePassword)
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
			log.Ctx(ctx).Info().Msg("Begining garbage collection")
			err := core.CollectGarbage(ctx, db, blobStore, time.Now().Add(-30*time.Minute).UTC())
			if err == nil {
				log.Ctx(ctx).Info().Msg("Garbage collection completed")
				break
			}

			log.Ctx(ctx).Error().Msgf("Garbage collection failed: %v", err)
			time.Sleep(30 * time.Second)
		}
	}
}

func configureZerolog(args Args) {

	zerolog.TimeFieldFormat = "2006-01-02T15:04:05.999Z07:00"

	var level zerolog.Level
	switch args.LogLevel {
	case "Trace":
		level = zerolog.TraceLevel
	case "Debug":
		level = zerolog.DebugLevel
	case "Info":
		level = zerolog.InfoLevel
	case "Warn":
		level = zerolog.WarnLevel
	case "Error":
		level = zerolog.ErrorLevel
	case "Fatal":
		level = zerolog.FatalLevel
	case "Panic":
		level = zerolog.PanicLevel
	case "Disabled":
		level = zerolog.Disabled
	}
	zerolog.SetGlobalLevel(level)

	if args.PrettyPrint {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	zerolog.DefaultContextLogger = &log.Logger
}

type ConfigSpec struct {
	DatabaseProvider         string `default:"sqlite"`
	DatabaseConnectionString string `default:"_data/metadata.db"`
	DatabasePassword         string
	StorageProvider          string `default:"filesystem"`
	StorageConnectionString  string `default:"_data/blobs"`
	Port                     int    `default:"3333"`
	LogRequests              bool   `default:"true"`
}
