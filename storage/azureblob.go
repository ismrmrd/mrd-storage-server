package storage

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"path"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/ismrmrd/mrd-storage-server/core"
	"github.com/rs/zerolog/log"
)

var (
	ErrInvalidConnectionString = errors.New("invalid connection string")
)

type azureBlobStore struct {
	containerClient azblob.ContainerClient
}

func NewAzureBlobStore(connectionString string) (core.BlobStore, error) {

	serviceClient, err := azblob.NewServiceClientFromConnectionString(connectionString, nil)
	if err != nil {
		return nil, err
	}
	containerClient := serviceClient.NewContainerClient("mrd-storage-server")
	if _, err := containerClient.Create(context.Background(), nil); err != nil {
		var storageError *azblob.StorageError
		if !errors.As(err, &storageError) || storageError.ErrorCode != azblob.StorageErrorCodeContainerAlreadyExists {
			return nil, err
		}
	}

	return &azureBlobStore{containerClient: containerClient}, nil
}

func (s *azureBlobStore) SaveBlob(ctx context.Context, contents io.Reader, key core.BlobKey) error {
	blobClient := s.containerClient.NewBlockBlobClient(blobName(key))
	_, err := blobClient.UploadStreamToBlockBlob(ctx, contents, azblob.UploadStreamToBlockBlobOptions{})
	return err
}

func (s *azureBlobStore) ReadBlob(ctx context.Context, writer io.Writer, key core.BlobKey) error {
	blobClient := s.containerClient.NewBlockBlobClient(blobName(key))
	resp, err := blobClient.Download(ctx, &azblob.DownloadBlobOptions{})
	if err != nil {
		var storageError *azblob.StorageError
		if errors.As(err, &storageError) && storageError.ErrorCode == azblob.StorageErrorCodeBlobNotFound {
			return core.ErrBlobNotFound
		}

		return err
	}
	reader := resp.Body(&azblob.RetryReaderOptions{MaxRetryRequests: 20})
	_, err = io.Copy(writer, reader)
	return err
}

func (s *azureBlobStore) DeleteBlob(ctx context.Context, key core.BlobKey) error {
	blobClient := s.containerClient.NewBlockBlobClient(blobName(key))
	if _, err := blobClient.Delete(ctx, &azblob.DeleteBlobOptions{}); err != nil {
		var storageError *azblob.StorageError
		if !errors.As(err, &storageError) || storageError.ErrorCode != azblob.StorageErrorCodeBlobNotFound {
			return err
		}
	}

	return nil
}

func (s *azureBlobStore) HealthCheck(ctx context.Context) error {
	_, err := s.containerClient.GetProperties(ctx, nil)
	if err != nil {
		log.Ctx(ctx).Error().Msgf("storage health check failed: %v", err)
		return errors.New("error accessing storage")
	}

	return nil
}

func blobName(key core.BlobKey) string {
	// make sure we don't have file names / or .. or anything like that
	encodedSubject := base64.RawURLEncoding.EncodeToString([]byte(key.Subject))
	return path.Join(encodedSubject, key.Id.String())
}
