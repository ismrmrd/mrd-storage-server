package api

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/gofrs/uuid"
	"github.com/ismrmrd/mrd-storage-api/core"
	log "github.com/sirupsen/logrus"
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
	subjectTagValidator = CombineTagValidators(ValidateSubjectTagValue, systemTagValidator)
)

type TagValidator func(tagName string, tagValues []string) error

func (handler *Handler) CreateBlob(w http.ResponseWriter, r *http.Request) {

	tags := core.BlobTags{}

	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		tags.ContentType = &contentType
	}

	query := normalizeQueryMapToLowercaseKeys(r.URL.Query())

	for tagName, v := range query {
		if err := ValidateAndStoreTag(&tags, tagName, v); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJson(w, r, CreateErrorResponse("InvalidTag", err.Error()))
			return
		}
	}

	if len(tags.Subject) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		writeJson(w, r, CreateErrorResponse("InvalidTag", fmt.Sprintf("The subject tag is missing and must be provided. If the no subject is associated with the blob, specify `%s`", NullSubject)))
		return
	}

	id, err := uuid.NewV4()
	if err != nil {
		log.Panic(err)
	}

	if err := handler.store.SaveBlob(r.Context(), r.Body, tags.Subject, id); err != nil {
		log.Errorf("Failed to save blob: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := handler.db.CreateBlobMetadata(r.Context(), id, &tags); err != nil {
		log.Errorf("Failed to write blob metadata to database: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Location", getBlobUri(r, tags.Subject, id))
	w.WriteHeader(http.StatusCreated)
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

func ValidateAndStoreOptionalSystemTag(tagName string, tagValues []string, field **string) error {
	if err := systemTagValidator(tagName, tagValues); err != nil {
		return err
	}

	*field = &tagValues[0]
	return nil
}

func ValidateAndStoreSubjectTag(tagName string, tagValues []string, field *string) error {
	if err := subjectTagValidator(tagName, tagValues); err != nil {
		return err
	}

	*field = tagValues[0]

	return nil
}

func ValidateAndStoreTag(tags *core.BlobTags, tagName string, tagValues []string) error {
	switch tagName {
	case "subject":
		return ValidateAndStoreSubjectTag(tagName, tagValues, &tags.Subject)
	case "device":
		return ValidateAndStoreOptionalSystemTag(tagName, tagValues, &tags.Device)
	case "name":
		return ValidateAndStoreOptionalSystemTag(tagName, tagValues, &tags.Name)
	case "session":
		return ValidateAndStoreOptionalSystemTag(tagName, tagValues, &tags.Session)
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
