package storage

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/gofrs/uuid"
	"github.com/ismrmrd/mrd-storage-api/core"
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

func (s fileSystemStore) SaveBlob(contents io.Reader, subject string, id uuid.UUID) error {

	filePath := s.filename(subject, id)

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

func (s fileSystemStore) ReadBlob(writer io.Writer, subject string, id uuid.UUID) error {
	filePath := s.filename(subject, id)

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}

	defer f.Close()
	reader := bufio.NewReader(f)
	_, err = io.Copy(writer, reader)
	return err
}

func (s fileSystemStore) filename(subject string, id uuid.UUID) string {
	// make sure we don't have file names / or .. or anything like that
	encodedSubject := base64.RawURLEncoding.EncodeToString([]byte(subject))
	return path.Join(s.rootDir, encodedSubject, id.String())
}
