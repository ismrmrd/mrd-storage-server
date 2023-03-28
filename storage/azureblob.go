package storage

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"path"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/ismrmrd/mrd-storage-server/core"
	"github.com/rs/zerolog/log"
)

var (
	ErrInvalidConnectionString = errors.New("invalid connection string")
)

type azureBlobStore struct {
	containerClient *container.Client
}

func NewAzureBlobStore(connectionString string) (core.BlobStore, error) {
	containerClient, err := container.NewClientFromConnectionString(connectionString, "mrd-storage-server", nil)
	if err != nil {
		return nil, err
	}

	if _, err := containerClient.Create(context.Background(), nil); err != nil {
		if !bloberror.HasCode(err, bloberror.ContainerAlreadyExists) {
			return nil, err
		}
	}

	return &azureBlobStore{containerClient: containerClient}, nil
}

func (s *azureBlobStore) SaveBlob(ctx context.Context, contents io.Reader, key core.BlobKey) error {
	blobClient := s.containerClient.NewBlockBlobClient(blobName(key))
	_, err := blobClient.UploadStream(ctx, contents, nil)
	return err
}

func (s *azureBlobStore) ReadBlob(ctx context.Context, writer io.Writer, key core.BlobKey) error {
	blobClient := s.containerClient.NewBlockBlobClient(blobName(key))
	resp, err := blobClient.DownloadStream(ctx, &blob.DownloadStreamOptions{})
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return core.ErrBlobNotFound
		}

		return err
	}
	_, err = io.Copy(writer, resp.Body)
	return err
}

func (s *azureBlobStore) DeleteBlob(ctx context.Context, key core.BlobKey) error {
	blobClient := s.containerClient.NewBlockBlobClient(blobName(key))
	if _, err := blobClient.Delete(ctx, &azblob.DeleteBlobOptions{}); err != nil {
		if !bloberror.HasCode(err, bloberror.BlobNotFound) {
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
