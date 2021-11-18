package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/gofrs/uuid"
	"github.com/ismrmrd/mrd-storage-server/core"
)

// use a dedicated type to avoid context key collisions
type contextKey int

const apiVersionContextKey contextKey = 0

type Handler struct {
	db    core.MetadataDatabase
	store core.BlobStore
}

func BuildRouter(db core.MetadataDatabase, store core.BlobStore, logRequests bool) http.Handler {
	handler := Handler{db: db, store: store}
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	if logRequests {
		r.Use(middleware.Logger)
	}
	r.Use(middleware.Recoverer)

	r.Route("/v1", func(r chi.Router) {
		r.Use(createApiVersionMiddleware("v1"))
		r.Route("/blobs", func(r chi.Router) {
			r.Post("/data", handler.CreateBlob)
			r.Get("/", handler.SearchBlobs)
			r.Get("/data/latest", handler.GetLatestBlobData)
			r.Get("/{combined-id}", handler.MakeBlobEndpoint(handler.BlobMetadataResponse))
			r.Get("/{combined-id}/data", handler.MakeBlobEndpoint(handler.BlobDataResponse))
		})
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	return r
}

// Creates a middleware handler for the given api version that stores the api version
// in the request context
func createApiVersionMiddleware(apiVersion string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), apiVersionContextKey, apiVersion)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TagHeaderName(tagName string) string {
	return TagHeaderPrefix + tagName
}

func normalizeQueryMapToLowercaseKeys(values url.Values) url.Values {
	// normalize and merge key parameter keys to lowercase
	allLowercase := true
	for k := range values {
		lowerK := strings.ToLower(k)
		if lowerK != k {
			allLowercase = false
		}
	}

	if allLowercase {
		return values
	}

	normalizedValues := make(map[string][]string)

	for k, v := range values {
		lowerK := strings.ToLower(k)
		normalizedValues[lowerK] = append(normalizedValues[lowerK], v...)
	}

	return normalizedValues
}

func getBlobCombinedId(key core.BlobKey) string {
	return fmt.Sprintf("%v-%s", key.Id, key.Subject)
}

func getBlobSubjectAndIdFromCombinedId(combinedId string) (key core.BlobKey, ok bool) {
	if len(combinedId) >= 37 {
		id, err := uuid.FromString(combinedId[:36])
		if err == nil {
			key.Id = id
			key.Subject = combinedId[37:]

			return key, true
		}
	}

	return
}

func getBaseUri(r *http.Request) url.URL {
	// TODO: respect X-Forwarded-Host (and related) headers
	url := *r.URL

	if r.TLS == nil {
		url.Scheme = "http"
	} else {
		url.Scheme = "https"
	}

	url.Host = r.Host

	url.RawQuery = ""

	// the root path segment should be the current api version
	url.Path = r.Context().Value(apiVersionContextKey).(string)
	return url
}

func getBlobUri(r *http.Request, key core.BlobKey) string {

	uri := getBaseUri(r)
	uri.Path = path.Join(uri.Path, "blobs", getBlobCombinedId(key))

	return uri.String()
}

func getDataUri(r *http.Request, key core.BlobKey) string {

	uri := getBaseUri(r)
	uri.Path = path.Join(uri.Path, "blobs", getBlobCombinedId(key), "data")

	return uri.String()
}

func CreateBlobInfo(r *http.Request, blob *core.BlobInfo) map[string]interface{} {

	info := make(map[string]interface{})
	info["lastModified"] = blob.CreatedAt.UTC().Format(time.RFC3339Nano)
	if blob.ExpiresAt != nil {
		info["expires"] = blob.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}

	info["subject"] = blob.Key.Subject
	if blob.Tags.ContentType != nil {
		info["contentType"] = blob.Tags.ContentType
	}
	if blob.Tags.Device != nil {
		info["device"] = blob.Tags.Device
	}
	if blob.Tags.Name != nil {
		info["name"] = blob.Tags.Name
	}
	if blob.Tags.Session != nil {
		info["session"] = blob.Tags.Session
	}
	info["location"] = getBlobUri(r, blob.Key)
	info["data"] = getDataUri(r, blob.Key)

	for k, v := range blob.Tags.CustomTags {
		if len(v) == 1 {
			info[k] = v[0]
		} else {
			info[k] = v
		}
	}

	return info
}

func writeJson(w http.ResponseWriter, r *http.Request, v interface{}) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(buf.Bytes())
}
