package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ismrmrd/mrd-storage-server/mocks"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	testCases := []struct {
		desc           string
		databaseErr    error
		storageErr     error
		expectedStatus int
	}{
		{"database error", errors.New("databaseErr"), nil, http.StatusServiceUnavailable},
		{"storage error", nil, errors.New("storageErr"), http.StatusServiceUnavailable},
		{"storage and database error", errors.New("databaseErr"), errors.New("storageErr"), http.StatusServiceUnavailable},
		{"no errors", nil, nil, http.StatusOK},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockMetadataDatabase := mocks.NewMockMetadataDatabase(mockCtrl)
			mockBlobStore := mocks.NewMockBlobStore(mockCtrl)

			mockMetadataDatabase.EXPECT().
				HealthCheck(gomock.Any()).
				Return(tc.databaseErr)

			mockBlobStore.EXPECT().
				HealthCheck(gomock.Any()).
				Return(tc.storageErr)

			handler := BuildRouter(mockMetadataDatabase, mockBlobStore, false)

			req := httptest.NewRequest("GET", "/healthcheck", nil)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			assert.Equal(t, tc.expectedStatus, resp.Result().StatusCode)
		})
	}
}
