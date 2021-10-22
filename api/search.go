package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ismrmrd/mrd-storage-api/core"
	"github.com/ismrmrd/mrd-storage-api/database"
	log "github.com/sirupsen/logrus"
)

func (handler *Handler) SearchBlobs(w http.ResponseWriter, r *http.Request) {

	query, at, ct, pageSize, ok := getSearchParameters(w, r)
	if !ok {
		return
	}

	results, ct, err := handler.db.SearchBlobMetadata(query, at, ct, pageSize)

	if err != nil {
		if errors.Is(err, database.ErrInvalidContinuationToken) {
			w.WriteHeader(http.StatusBadRequest)
			writeJson(w, r, CreateErrorResponse("InvalidContinuationToken", "The'_ct' parameter is invalid."))
			return
		}

		log.Errorf("Failed to search blobs in DB: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	responseEntries := make([]map[string]interface{}, len(results))

	for i, res := range results {
		entry := make(map[string]interface{})
		entry["lastModified"] = res.CreatedAt.Format(time.RFC3339Nano)

		entry["subject"] = res.Tags.Subject
		if res.Tags.ContentType != nil {
			entry["contentType"] = res.Tags.ContentType
		}
		if res.Tags.Device != nil {
			entry["device"] = res.Tags.Device
		}
		if res.Tags.Name != nil {
			entry["name"] = res.Tags.Name
		}
		if res.Tags.Session != nil {
			entry["session"] = res.Tags.Session
		}
		entry["location"] = getBlobUri(r, res.Tags.Subject, res.Id)

		for k, v := range res.Tags.CustomTags {
			if len(v) == 1 {
				entry[k] = v[0]
			} else {
				entry[k] = v
			}
		}

		responseEntries[i] = entry
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

func (handler *Handler) GetLatestBlob(w http.ResponseWriter, r *http.Request) {

	query, at, _, _, ok := getSearchParameters(w, r)
	if !ok {
		return
	}

	results, _, err := handler.db.SearchBlobMetadata(query, at, nil, 1)

	if err != nil {
		log.Errorf("Failed to search blobs in DB: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(results) == 0 {
		w.WriteHeader(http.StatusNotFound)
		writeJson(w, r, CreateErrorResponse("EmptyResults", "The search returned no results."))
		return
	}

	latestBlobInfo := results[0]

	w.Header().Add("Location", getBlobUri(r, latestBlobInfo.Tags.Subject, latestBlobInfo.Id))

	handler.BlobResponse(w, &latestBlobInfo)
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
