package storage

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/ismrmrd/mrd-storage-server/core"
	log "github.com/sirupsen/logrus"
)

type fileSystemStore struct {
	rootDir string
}

func NewFileSystemStore(rootDir string) (core.BlobStore, error) {
	if err := os.Mkdir(rootDir, os.ModePerm); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("unable to create directory: %v", err)
	}

	return fileSystemStore{rootDir: rootDir}, nil
}

func (s fileSystemStore) SaveBlob(ctx context.Context, contents io.Reader, key core.BlobKey) error {

	filePath := s.filename(key)

	if err := os.Mkdir(path.Dir(filePath), os.ModePerm); err != nil && !os.IsExist(err) {
		return fmt.Errorf("unable to create directory: %v", err)
	}

	f, err := os.Create(filePath)
	if err != nil {
		return err
	}

	defer f.Close()
	writer := bufio.NewWriter(f)
	_, err = io.Copy(writer, contents)
	return err
}

func (s fileSystemStore) ReadBlob(ctx context.Context, writer io.Writer, key core.BlobKey) error {
	filePath := s.filename(key)

	f, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return core.ErrBlobNotFound
		}
		return err
	}

	defer f.Close()
	reader := bufio.NewReader(f)
	_, err = io.Copy(writer, reader)
	return err
}

func (s fileSystemStore) DeleteBlob(ctx context.Context, key core.BlobKey) error {
	filePath := s.filename(key)

	if err := os.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}

func (s fileSystemStore) HealthCheck(ctx context.Context) error {
	_, err := os.Stat(s.rootDir)
	if err != nil {
		log.Errorf("filesystem health check failed: %v", err)
		return errors.New("error accessing storage")
	}
	return nil
}

func (s fileSystemStore) filename(key core.BlobKey) string {
	// make sure we don't have file names / or .. or anything like that
	encodedSubject := base64.RawURLEncoding.EncodeToString([]byte(key.Subject))
	return path.Join(s.rootDir, encodedSubject, key.Id.String())
}
