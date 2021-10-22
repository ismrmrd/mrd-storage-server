package api

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/golang/mock/gomock"
	"github.com/ismrmrd/mrd-storage-api/mocks"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestStorageWriteFailureSkipsMetadataWrite(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockMetadataDatabase := mocks.NewMockMetadataDatabase(mockCtrl) // will fail if called
	mockBlobStore := mocks.NewMockBlobStore(mockCtrl)

	mockBlobStore.EXPECT().
		SaveBlob(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("failed to write"))

	handler := Handler{db: mockMetadataDatabase, store: mockBlobStore}

	req := httptest.NewRequest("POST", "/blob?subject=a", strings.NewReader("content"))
	resp := httptest.NewRecorder()

	handler.CreateBlob(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Result().StatusCode)
}
