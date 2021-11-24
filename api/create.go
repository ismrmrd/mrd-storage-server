package api

import (
	"errors"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/ismrmrd/mrd-storage-server/core"
	log "github.com/sirupsen/logrus"
	"net/http"
	"regexp"
	"time"
)

const (
	TagHeaderPrefix = "Mrd-Tag-"
	NullSubject     = "$null"
)

var (
	reservedTagNames = map[string]bool{
		"location":      true,
		"last-modified": true,
		"lastmodified":  true,
		"content-type":  true,
		"contenttype":   true,
	}
	tagNameRegex, _     = regexp.Compile(`(^[a-zA-Z][a-zA-Z0-9_\-]{0,63}$)|$null`)
	commonTagValidator  = CombineTagValidators(ValidateTagName, ValidateGenericTagValues)
	systemTagValidator  = CombineTagValidators(commonTagValidator, ValidateOnlyOneTag)
	ttlTagValidator     = CombineTagValidators(ValidateTimeToLive, systemTagValidator)
	subjectTagValidator = CombineTagValidators(ValidateSubjectTagValue, systemTagValidator)
)

type TagValidator func(tagName string, tagValues []string) error

func (handler *Handler) CreateBlob(w http.ResponseWriter, r *http.Request) {

	id, err := uuid.NewV4()
	if err != nil {
		log.Panic(err)
	}

	key := core.BlobKey{Id: id}

	query := normalizeQueryMapToLowercaseKeys(r.URL.Query())

	if subjectStrings, ok := query["subject"]; !ok {
		w.WriteHeader(http.StatusBadRequest)
		writeJson(w, r, CreateErrorResponse("InvalidTag", fmt.Sprintf("The subject tag is missing and must be provided. If the no subject is associated with the blob, specify `%s`", NullSubject)))
		return
	} else {
		if err := subjectTagValidator("subject", subjectStrings); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJson(w, r, CreateErrorResponse("InvalidSubject", err.Error()))
			return
		}
		key.Subject = subjectStrings[0]
		delete(query, "subject")
	}

	tags := core.BlobTags{}

	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		tags.ContentType = &contentType
	}

	for tagName, v := range query {
		if err := ValidateAndStoreTag(&tags, tagName, v); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJson(w, r, CreateErrorResponse("InvalidTag", err.Error()))
			return
		}
	}

	blobInfo, err := handler.db.StageBlobMetadata(r.Context(), key, &tags)
	if err != nil {
		log.Errorf("Failed to stage blob metadata: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := handler.store.SaveBlob(r.Context(), r.Body, key); err != nil {
		log.Errorf("Failed to save blob: %v", err)

		err = handler.db.DeleteBlobMetadata(r.Context(), key)
		if err != nil {
			log.Errorf("Failed to revert staged blob metadata: %v", err)
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := handler.db.CompleteStagedBlobMetadata(r.Context(), key); err != nil {
		log.Errorf("Failed to complete staged metadata to database: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJson(w, r, CreateBlobInfo(r, blobInfo))
}

func ValidateTagName(tagName string, tagValues []string) error {
	if reservedTagNames[tagName] {
		return fmt.Errorf("tag name '%s' is reserved", tagName)
	}

	if !tagNameRegex.MatchString(tagName) {
		return fmt.Errorf("tag name '%s' is invalid: it must be '$null' or start with a letter a-z, followed by up to 63 letters (a-z), numbers, hyphens, or underscores", tagName)
	}

	return nil
}

func ValidateOnlyOneTag(tagName string, tagValues []string) error {
	if len(tagValues) != 1 {
		return fmt.Errorf("only one value for tag '%s' can be given", tagName)
	}

	return nil
}

func ValidateGenericTagValues(tagName string, tagValues []string) error {

	for _, t := range tagValues {
		if len(t) > 128 {
			return fmt.Errorf("the value for tag '%s' is longer than 128 UTF-8 characters", tagName)
		}
	}

	return nil
}

func ValidateSubjectTagValue(tagName string, tagValues []string) error {

	if tagValues[0] == "" {
		return errors.New("the subject tag cannot be empty")
	}

	return nil
}

func ValidateTimeToLive(tagName string, tagValues []string) error {

	for _, t := range tagValues {
		if _, err := time.ParseDuration(t); err != nil {
			return err
		}
	}

	return nil
}

func CombineTagValidators(validators ...TagValidator) TagValidator {
	return func(tagName string, tagValues []string) error {
		for _, v := range validators {
			if err := v(tagName, tagValues); err != nil {
				return err
			}
		}

		return nil
	}
}

func ValidateAndStoreOptionalSystemTag(tagName string, tagValues []string, field **string, validator TagValidator) error {
	if err := validator(tagName, tagValues); err != nil {
		return err
	}

	*field = &tagValues[0]
	return nil
}

func ValidateAndStoreTag(tags *core.BlobTags, tagName string, tagValues []string) error {
	switch tagName {
	case "device":
		return ValidateAndStoreOptionalSystemTag(tagName, tagValues, &tags.Device, systemTagValidator)
	case "name":
		return ValidateAndStoreOptionalSystemTag(tagName, tagValues, &tags.Name, systemTagValidator)
	case "session":
		return ValidateAndStoreOptionalSystemTag(tagName, tagValues, &tags.Session, systemTagValidator)
	case "ttl":
		return ValidateAndStoreOptionalSystemTag(tagName, tagValues, &tags.TimeToLive, ttlTagValidator)
	default:
		if err := commonTagValidator(tagName, tagValues); err != nil {
			return err
		}
		if tags.CustomTags == nil {
			tags.CustomTags = make(map[string][]string)
		}
		tags.CustomTags[tagName] = tagValues
	}

	return nil
}
