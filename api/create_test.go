package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
	"github.com/ismrmrd/mrd-storage-server/mocks"
)

func init() {
	log.Logger = log.Output(io.Discard)
}

func TestStorageWriteFailureRevertsStagedMetadata(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockMetadataDatabase := mocks.NewMockMetadataDatabase(mockCtrl)
	mockBlobStore := mocks.NewMockBlobStore(mockCtrl)

	mockMetadataDatabase.EXPECT().
		StageBlobMetadata(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	mockMetadataDatabase.EXPECT().
		DeleteBlobMetadata(gomock.Any(), gomock.Any()).
		Return(nil)

	mockBlobStore.EXPECT().
		SaveBlob(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("failed to write"))

	handler := Handler{db: mockMetadataDatabase, store: mockBlobStore}

	req := httptest.NewRequest("POST", "/v1/blobs?subject=a", strings.NewReader("content"))
	resp := httptest.NewRecorder()

	handler.CreateBlob(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Result().StatusCode)
}

func TestStagingFailureResultsInAbortedRequest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockMetadataDatabase := mocks.NewMockMetadataDatabase(mockCtrl)
	mockBlobStore := mocks.NewMockBlobStore(mockCtrl)

	mockMetadataDatabase.EXPECT().
		StageBlobMetadata(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("failed to write to database"))

	handler := Handler{db: mockMetadataDatabase, store: mockBlobStore}

	req := httptest.NewRequest("POST", "/v1/blobs?subject=a", strings.NewReader("content"))
	resp := httptest.NewRecorder()

	handler.CreateBlob(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Result().StatusCode)
}
