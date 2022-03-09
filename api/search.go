package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ismrmrd/mrd-storage-server/core"
	"github.com/rs/zerolog/log"
)

func (handler *Handler) SearchBlobs(w http.ResponseWriter, r *http.Request) {

	query, at, ct, pageSize, ok := getSearchParameters(w, r)
	if !ok {
		return
	}

	results, ct, err := handler.db.SearchBlobMetadata(r.Context(), query, at, ct, pageSize, time.Now())

	if err != nil {
		if errors.Is(err, core.ErrInvalidContinuationToken) {
			w.WriteHeader(http.StatusBadRequest)
			writeJson(w, r, CreateErrorResponse("InvalidContinuationToken", "The'_ct' parameter is invalid."))
			return
		}

		log.Ctx(r.Context()).Error().Msgf("Failed to search blobs in DB: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	responseEntries := make([]map[string]interface{}, len(results))

	for i, res := range results {
		responseEntries[i] = CreateBlobInfo(r, &res)
	}

	searchResponse := SearchResponse{Items: responseEntries}

	if ct != nil {
		nextQuery := r.URL.Query()
		nextQuery.Set("_ct", string(*ct))

		url := getBaseUri(r)
		url.Path = r.URL.Path
		url.RawQuery = nextQuery.Encode()
		searchResponse.NextLink = url.String()
	}

	writeJson(w, r, searchResponse)
}

func (handler *Handler) GetLatestBlobData(w http.ResponseWriter, r *http.Request) {

	query, at, _, _, ok := getSearchParameters(w, r)
	if !ok {
		return
	}

	results, _, err := handler.db.SearchBlobMetadata(r.Context(), query, at, nil, 1, time.Now())

	if err != nil {
		log.Ctx(r.Context()).Error().Msgf("Failed to search blobs in DB: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(results) == 0 {
		w.WriteHeader(http.StatusNotFound)
		writeJson(w, r, CreateErrorResponse("EmptyResults", "The search returned no results."))
		return
	}

	latestBlobInfo := results[0]

	w.Header().Add("Location", getBlobUri(r, latestBlobInfo.Key))

	handler.BlobDataResponse(w, r, &latestBlobInfo)
}

func getSearchParameters(w http.ResponseWriter, r *http.Request) (tags url.Values, at *time.Time, ct *core.ContinutationToken, pageSize int, ok bool) {
	tags = normalizeQueryMapToLowercaseKeys(r.URL.Query())

	if !tags.Has("subject") {
		w.WriteHeader(http.StatusBadRequest)
		writeJson(w, r, CreateErrorResponse("InvalidQuery", "'subject' query parameter is mandatory. To search for blobs where not associated with a subject, you can specify 'subject=$null'"))
		return
	}

	if timeStrings, hasAt := tags["_at"]; hasAt {
		if len(timeStrings) > 1 {
			w.WriteHeader(http.StatusBadRequest)
			writeJson(w, r, CreateErrorResponse("InvalidParameter", "The '_at' parameter was specified multiple times in the URL."))
			return
		}

		parsedTime, atErr := time.Parse(time.RFC3339Nano, timeStrings[0])
		if atErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJson(w, r, CreateErrorResponse("InvalidParameter", "The format of the '_at' parameter is invalid"))
			return
		}

		at = &parsedTime
		delete(tags, "_at")
	}

	pageSize = 100

	if limitStrings, hasLimit := tags["_limit"]; hasLimit {

		pageSize, _ = strconv.Atoi(limitStrings[0])
		if pageSize > 100 || pageSize <= 0 {
			pageSize = 100
		}

		delete(tags, "_limit")
	}

	if cts, hasCt := tags["_ct"]; hasCt {
		if len(cts) > 1 {
			w.WriteHeader(http.StatusBadRequest)
			writeJson(w, r, CreateErrorResponse("InvalidContinuationToken", "The'_ct' parameter was specified multiple times in the URL."))
			return
		}

		ct = (*core.ContinutationToken)(&cts[0])
		delete(tags, "_ct")
	}

	ok = true
	return
}
