package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ismrmrd/mrd-storage-server/core"
	log "github.com/sirupsen/logrus"
	"github.com/xorcare/pointer"
)

type Responder func(http.ResponseWriter, *http.Request, *core.BlobInfo)

func (handler *Handler) MakeBlobEndpoint(responder Responder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		combinedId := chi.URLParam(r, "combined-id")
		key, ok := getBlobSubjectAndIdFromCombinedId(combinedId)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		blobInfo, err := handler.db.GetBlobMetadata(r.Context(), key)
		if err != nil {
			if errors.Is(err, core.ErrRecordNotFound) {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			log.Errorf("Database read failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		responder(w, r, blobInfo)
	}
}

func (handler *Handler) BlobMetadataResponse(w http.ResponseWriter, r *http.Request, blobInfo *core.BlobInfo) {
	writeJson(w, r, CreateBlobInfo(r, blobInfo))
}

func (handler *Handler) BlobDataResponse(w http.ResponseWriter, r *http.Request, blobInfo *core.BlobInfo) {

	writeTagsAsHeaders(w, blobInfo)

	if err := handler.store.ReadBlob(r.Context(), w, blobInfo.Key); err != nil {
		log.Errorf("Failed to read blob from storage: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func writeTagsAsHeaders(w http.ResponseWriter, blobInfo *core.BlobInfo) {
	if blobInfo.Tags.ContentType == nil {
		blobInfo.Tags.ContentType = pointer.String("application/octet-stream")
	}
	w.Header().Add("Content-Type", *blobInfo.Tags.ContentType)

	w.Header().Add("Last-Modified", blobInfo.CreatedAt.Format(http.TimeFormat))

	addSystemTagIfSet(w, "Device", blobInfo.Tags.Device)
	addSystemTagIfSet(w, "Name", blobInfo.Tags.Name)
	addSystemTagIfSet(w, "Session", blobInfo.Tags.Session)
	addSystemTagIfSet(w, "Subject", &blobInfo.Key.Subject)

	for tagName, tagValues := range blobInfo.Tags.CustomTags {
		// Performing Add() on each entry instead of direcly assigning to the map
		// to that the casing is normalized the same way like in other headers
		for _, tagValue := range tagValues {
			w.Header().Add(TagHeaderName(tagName), tagValue)
		}
	}
}

func addSystemTagIfSet(w http.ResponseWriter, tagName string, tagValue *string) {
	if tagValue != nil {
		w.Header().Add(TagHeaderName(tagName), *tagValue)
	}
}
