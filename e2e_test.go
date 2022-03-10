package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	gourl "net/url"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ismrmrd/mrd-storage-server/api"
	"github.com/ismrmrd/mrd-storage-server/core"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	db        core.MetadataDatabase
	blobStore core.BlobStore
	router    http.Handler
	remoteUrl *gourl.URL
)

func init() {
	log.Logger = log.Output(io.Discard)

	if remoteUrlVar := os.Getenv("TEST_REMOTE_URL"); remoteUrlVar != "" {
		var err error
		remoteUrl, err = gourl.Parse(remoteUrlVar)
		if err != nil {
			log.Fatal().Msg("Invalid TEST_REMOTE_URL value")
		}

		return
	}

	// Put the server in a non-UTC time zone so that we
	// can verify that times are always returned in UTC and not the server's time zone.
	time.Local = time.FixedZone("MyTimeZone", 3600)

	config := loadConfig()
	config.LogRequests = false

	dbProvider := os.Getenv("TEST_DB_PROVIDER")

	switch dbProvider {
	case ConfigDatabaseProviderPostgresql:
		config.DatabaseProvider = ConfigDatabaseProviderPostgresql
		config.DatabaseConnectionString = "user=mrd password=mrd dbname=mrd host=localhost port=9920 sslmode=disable"
	case "", ConfigDatabaseProviderSqlite:
		config.DatabaseConnectionString = "./_data/metadata.db"
	default:
		log.Fatal().Msgf("Unrecognized TEST_DB_PROVIDER environment variable '%s'", dbProvider)
	}

	storageProvider := os.Getenv("TEST_STORAGE_PROVIDER")

	switch storageProvider {
	case ConfigStorageProviderAzureBlob:
		config.StorageProvider = ConfigStorageProviderAzureBlob
		config.StorageConnectionString = "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://localhost:10000/devstoreaccount1;"
	case "", ConfigStorageProviderFileSystem:
		config.StorageConnectionString = "./_data/blobs"
	default:
		log.Fatal().Msgf("Unrecognized TEST_STORAGE_PROVIDER environment variable '%s'", storageProvider)
	}

	var err error
	db, blobStore, err = assembleDataStores(config)
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	router = assembleHandler(db, blobStore, config)
}

func TestInvalidTags(t *testing.T) {

	cases := []struct {
		name  string
		query string
	}{
		{"tag name with leading underscore", "subject=s&_r=s"},
		{"tag name with unsupported char", "subject=s&a*=s"},
		{"tag name that is too long", fmt.Sprintf("subject=sub&%s=abc", strings.Repeat("a", 65))},
		{"Location", "subject=s&location=l"},
		{"Last-Modified", "subject=s&lastModified=2021-10-18T16:56:15.693Z"},
		{"bad ttl", "subject=s&_ttl=not-an-interval"},
		{"negative ttl", "subject=s&_ttl=-1h"},
		{"Many Subject tags", "subject=s&subject=s2"},
		{"Subject empty", "subject="},
		{"No subject tag", ""},
		{"Many Device tags", "subject=s&device=d1&device=d2"},
		{"Many Name tags", "subject=s&name=n1&name=n2"},
		{"Many Session tags", "subject=s&session=s1&session=s2"},
		{"Tag value too long", fmt.Sprintf("subject=sub&a=%s", strings.Repeat("a", 200))},
	}

	for _, c := range cases {

		t.Run(c.name, func(t *testing.T) {
			r := create(t, c.query, "text-plain", "hello")
			assert.Equal(t, http.StatusBadRequest, r.StatusCode)
			assert.NotNil(t, r.ErrorResponse)
		})
	}

}

func TestCreateValidBlob(t *testing.T) {

	bodyContents := "this is the body"

	subject := fmt.Sprint(time.Now().UnixNano())

	// Create the blob
	createResp := create(t, fmt.Sprintf("subject=%s&name=myname&device=mydevice", subject), "", bodyContents)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	// now read the blob using the Location header in the response

	readResp := read(t, createResp.Data)
	assert.Equal(t, http.StatusOK, readResp.StatusCode)
	assert.Equal(t, bodyContents, readResp.Body)
	assert.Equal(t, "application/octet-stream", *readResp.Tags.ContentType)
	assert.NotNil(t, readResp.CreatedAt)
	assert.Equal(t, subject, readResp.Subject)
	assert.Equal(t, "myname", *readResp.Tags.Name)
	assert.Equal(t, "mydevice", *readResp.Tags.Device)
	assert.Nil(t, readResp.Tags.Session)

	searchResp := search(t, "subject="+subject)
	require.Equal(t, http.StatusOK, searchResp.StatusCode)
	assert.Len(t, searchResp.Results.Items, 1)
}

func TestCreateValidBlobCustomTags(t *testing.T) {

	bodyContents := "this is the body"

	subject := fmt.Sprint(time.Now().UnixNano())

	// Create the blob
	createResp := create(
		t,
		fmt.Sprintf("subject=%s&session=mysession&customtag1=customTag1Value&customTag2=customTag2Value1&customTag2=customTag2Value2", subject),
		"text/plain",
		bodyContents)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	readResp := read(t, createResp.Data)
	assert.Equal(t, http.StatusOK, readResp.StatusCode)
	assert.Equal(t, bodyContents, readResp.Body)

	assert.Equal(t, "text/plain", *readResp.Tags.ContentType)
	assert.NotNil(t, readResp.CreatedAt)
	assert.Equal(t, subject, readResp.Subject)
	assert.Equal(t, "mysession", *readResp.Tags.Session)

	assert.ElementsMatch(t, []string{"customTag1Value"}, readResp.Tags.CustomTags["Customtag1"])
	assert.ElementsMatch(t, []string{"customTag2Value1", "customTag2Value2"}, readResp.Tags.CustomTags["Customtag2"])

	searchResp := search(t, fmt.Sprintf("subject=%s&CustomTag2=customTag2Value1", subject))
	assert.Equal(t, http.StatusOK, searchResp.StatusCode)
	assert.Len(t, searchResp.Results.Items, 1)

	searchResp = search(t, fmt.Sprintf("subject=%s&CustomTag2=customTag2Value1&CustomTag2=missing", subject))
	assert.Equal(t, http.StatusOK, searchResp.StatusCode)
	assert.Empty(t, searchResp.Results.Items)

	searchResp = search(t, fmt.Sprintf("subject=%s&CustomTag2=customTag2Value1&CustomTag2=customTag2Value2", subject))
	assert.Equal(t, http.StatusOK, searchResp.StatusCode)
	assert.Len(t, searchResp.Results.Items, 1)
}

func TestCreateValidBlobTimeToLive(t *testing.T) {

	body := "This is a body."
	subject := fmt.Sprint(time.Now().UnixNano())
	ttl := "10m9s" // 10 minutes, 9 seconds
	now := time.Now()

	response := create(
		t,
		fmt.Sprintf("subject=%s&session=mysession&_ttl=%s", subject, ttl),
		"text/plain",
		body)

	require.Equal(t, http.StatusCreated, response.StatusCode)
	assert.Equal(t, subject, response.Meta["subject"])
	assert.Equal(t, "mysession", response.Meta["session"])
	require.NotNil(t, response.Meta["expires"])
	assert.Regexp(t, "Z$", response.Meta["expires"], "Expiration datetime in response not in UTC")

	duration, _ := time.ParseDuration(ttl)
	expected := now.Add(duration)

	// Check that the expiration is approximately 10 minutes, 9 seconds in the future.
	expires, err := time.Parse(time.RFC3339Nano, response.Meta["expires"].(string))
	require.Nil(t, err)

	almostEqual := func(a time.Time, b time.Time) bool {
		return math.Abs(a.Sub(b).Seconds()) < 500e-3
	}

	require.True(t, almostEqual(expected, expires))
}

func TestExpiredBlobNotInSearchResults(t *testing.T) {

	body := "An expired blob should not be displayed in search results."
	subject := fmt.Sprint(time.Now().UnixNano())

	for _, ttl := range []string{"0s", "10m", "10m"} {
		response := create(
			t,
			fmt.Sprintf("subject=%s&session=expiration-search-test&_ttl=%s&expiration-test=%s", subject, ttl, ttl),
			"text/plain",
			body)
		require.Equal(t, http.StatusCreated, response.StatusCode)
	}

	time.Sleep(50 * time.Millisecond)

	response := search(t, fmt.Sprintf("subject=%s&session=expiration-search-test", subject))

	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, 2, len(response.Results.Items))

	for _, item := range response.Results.Items {
		require.Equal(t, "10m", item["expiration-test"].(string))
	}
}

func TestExpiredBlobNotFound(t *testing.T) {

	body := "Getting an expired blob directly should return 404"
	subject := fmt.Sprint(time.Now().UnixNano())

	response := create(
		t,
		fmt.Sprintf("subject=%s&session=expiration-lookup-test&_ttl=0s", subject),
		"text/plain",
		body)

	require.Equal(t, http.StatusCreated, response.StatusCode)
	require.Contains(t, response.Meta, "expires")

	time.Sleep(50 * time.Millisecond)

	response = get(t, response.Location)

	require.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestBlobCreatedWithTimeToLiveHasExpiration(t *testing.T) {

	body := "This is a body. It will do nicely."
	subject := fmt.Sprint(time.Now().UnixNano())
	ttl := "45m"

	createResponse := create(
		t,
		fmt.Sprintf("subject=%s&session=mysession&_ttl=%s", subject, ttl),
		"text/plain",
		body)
	require.Equal(t, http.StatusCreated, createResponse.StatusCode)

	readResponse := get(t, createResponse.Location)
	require.Equal(t, http.StatusOK, readResponse.StatusCode)
	require.NotNil(t, readResponse.Meta["expires"])
	require.Equal(t, createResponse.Meta["expires"], readResponse.Meta["expires"])
}

func TestBlobCreateWithTimeToLiveHasExpirationDataHeader(t *testing.T) {
	body := "This is a body. Just bytes really."
	subject := fmt.Sprint(time.Now().UnixNano())
	ttl := "5m"

	createResponse := create(
		t,
		fmt.Sprintf("subject=%s&session=mysession&_ttl=%s", subject, ttl),
		"text/plain",
		body)
	require.Equal(t, http.StatusCreated, createResponse.StatusCode)

	data := read(t, createResponse.Data)
	require.NotNil(t, data.ExpiresAt)
}

func TestCreateResponse(t *testing.T) {

	body := "these are some bytes"
	subject := "$null"

	response := create(t, fmt.Sprintf("subject=%s&session=mysession", subject), "text/plain", body)
	require.Equal(t, http.StatusCreated, response.StatusCode)

	assert.NotNil(t, response.Meta["lastModified"])
	assert.Equal(t, "text/plain", response.Meta["contentType"])

	assert.Equal(t, "mysession", response.Meta["session"])
	assert.Equal(t, "$null", response.Meta["subject"])
	assert.Nil(t, response.Meta["name"])

	assert.NotNil(t, response.Meta["location"])
	assert.NotNil(t, response.Meta["data"])
}

func TestCreateResponseCustomTags(t *testing.T) {

	body := "these are some bytes"
	subject := fmt.Sprint(time.Now().UnixNano())

	// Create the blob
	response := create(
		t,
		fmt.Sprintf("subject=%s&session=mysession&customtag1=customTag1Value&customTag2=customTag2Value1&customTag2=customTag2Value2", subject),
		"text/plain",
		body)
	require.Equal(t, http.StatusCreated, response.StatusCode)

	require.Equal(t, "customTag1Value", response.Meta["customtag1"])
	require.ElementsMatch(t, []string{"customTag2Value1", "customTag2Value2"}, response.Meta["customtag2"])
}

func TestCreateResponseMatchesBlobMeta(t *testing.T) {

	body := "these are some bytes"
	subject := "$null"

	createResponse := create(t, fmt.Sprintf("subject=%s&session=mysession", subject), "text/plain", body)
	require.Equal(t, http.StatusCreated, createResponse.StatusCode)

	readResponse := get(t, createResponse.Location)

	require.Equal(t, http.StatusOK, readResponse.StatusCode)
	require.True(t, reflect.DeepEqual(createResponse.Meta, readResponse.Meta))
}

func TestSearchPaging(t *testing.T) {

	subject := fmt.Sprint(time.Now().UnixNano())

	// create several blobs with the same subject
	totalItems := 10
	originalQuery := fmt.Sprintf("subject=%s&mytag=t", subject)
	for i := 0; i < totalItems; i++ {
		require.Equal(t, http.StatusCreated, create(t, originalQuery, "", "").StatusCode)
	}

	for _, pageSize := range []int{3, 5, 8, 10, 11} {

		t.Run(fmt.Sprintf("page size %d", pageSize), func(t *testing.T) {

			link := fmt.Sprintf("/v1/blobs?subject=%s&mytag=t&_limit=%d", subject, pageSize)

			items := make(map[string]bool)
			for link != "" {

				resp := search(t, link[strings.Index(link, "?")+1:])

				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.LessOrEqual(t, len(resp.Results.Items), pageSize)

				for _, v := range resp.Results.Items {
					location := v["location"].(string)
					assert.NotContains(t, items, location)
					items[string(location)] = true
				}

				link = resp.Results.NextLink
			}

			assert.Equal(t, len(items), totalItems)
		})
	}

	// now verify the behavior of the _at parameter for searches and get latest calls
	fullResults := search(t, originalQuery+"&_limit=0") // <= 0 should be ignored
	assert.Empty(t, fullResults.Results.NextLink)

	for i := 1; i < len(fullResults.Results.Items); i++ {
		previousResult := fullResults.Results.Items[i-1]
		thisResult := fullResults.Results.Items[i]
		if prevTime, thisTime := previousResult["lastModified"].(string), thisResult["lastModified"].(string); prevTime != thisTime {
			assert.Regexp(t, "Z$", thisTime, "Datetime in response not in UTC")
			atQuery := fmt.Sprintf("%s&_at=%s", originalQuery, gourl.QueryEscape(thisTime))
			atRes := search(t, atQuery)

			require.Equal(t, http.StatusOK, atRes.StatusCode)
			assert.Equal(t, thisResult["location"].(string), atRes.Results.Items[0]["location"].(string))

			latestResponse := getLatestBlob(t, atQuery)
			require.Equal(t, http.StatusOK, latestResponse.StatusCode)
			assert.Equal(t, thisResult["location"].(string), latestResponse.Location)
		}
	}
}

func TestInvalidSearches(t *testing.T) {
	cases := []string{
		"a=a",
		"subject=x&_ct=3",
		"subject=x&_ct=_ct=eyJ0cyI6MTYzNDU3NjE3NzA4MH0&_ct=eyJ0cyI6MTYzNDU3NjE3NzA4MH0",
		"subject=x&_at=foobar",
		"subject=x&_at=2021",
		"subject=x&_at=2021-10-18T16:56:15.693Z&_at=2021-10-18T16:56:15.693Z",
	}

	for _, c := range cases {

		t.Run(c, func(t *testing.T) {
			resp := search(t, c)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestTagCaseSensitivity(t *testing.T) {
	subject := fmt.Sprintf("S-%d", time.Now().UnixNano())
	query := fmt.Sprintf("subject=%s&name=MYNAME&mytag=TAGVALUE1&MYTAG=TAGVALUE2", subject)
	createResp := create(t, query, "", "")
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	assert.Empty(t, search(t, strings.ToLower(query)).Results.Items)
	assert.NotEmpty(t, search(t, fmt.Sprintf("subject=%s&name=MYNAME&mytag=TAGVALUE1", subject)).Results.Items)
	assert.NotEmpty(t, search(t, fmt.Sprintf("SUBJECT=%s&name=MYNAME&mytag=TAGVALUE1", subject)).Results.Items)
	assert.NotEmpty(t, search(t, fmt.Sprintf("subject=%s&name=MYNAME&MYTAG=TAGVALUE1", subject)).Results.Items)
	assert.Empty(t, search(t, fmt.Sprintf("subject=%s&name=MYNAME&mytag=TAGVALUE1", strings.ToLower(subject))).Results.Items)
	assert.Empty(t, search(t, fmt.Sprintf("subject=%s&name=MYNAME&mytag=tagvalue1", subject)).Results.Items)
}

func TestUnicodeTags(t *testing.T) {
	subject := fmt.Sprintf("S-%d", time.Now().UnixNano())
	query := fmt.Sprintf("subject=%s&name=😁&mytag=😀", subject)

	createResp := create(t, query, "", "")
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	items := search(t, query).Results.Items
	require.NotEmpty(t, items)
	assert.Equal(t, "😁", items[0]["name"].(string))
	assert.Equal(t, "😀", items[0]["mytag"].(string))

	readResponse := read(t, createResp.Data)
	assert.Equal(t, "😁", *readResponse.Tags.Name)
	assert.Equal(t, "😀", readResponse.Tags.CustomTags["Mytag"][0])
}

func Test404(t *testing.T) {
	cases := []string{
		"/",
		fmt.Sprintf("/v1/blobs/latest?subject=%d", time.Now().UnixNano()),
		"/v1/blobs/abc",
	}

	for _, c := range cases {

		t.Run(c, func(t *testing.T) {
			resp, err := executeRequest("GET", c, nil, nil)
			require.Nil(t, err)
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}

func TestNullSubject(t *testing.T) {
	device := fmt.Sprint(time.Now().UnixNano())
	query := "subject=$null&device=" + device
	createResp := create(t, query, "", "hello")
	readResp := read(t, createResp.Data)
	assert.Equal(t, "hello", readResp.Body)

	latestResp := getLatestBlob(t, query)
	assert.Equal(t, "hello", latestResp.Body)
}

func TestHealthCheck(t *testing.T) {
	resp, err := executeRequest("GET", "/healthcheck", nil, nil)
	require.Nil(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func search(t *testing.T, queryString string) SearchResponse {
	resp, err := executeRequest("GET", fmt.Sprintf("/v1/blobs?%s", queryString), nil, nil)
	require.Nil(t, err)

	searchResponse := SearchResponse{}
	searchResponse.RawResponse = resp
	searchResponse.StatusCode = resp.StatusCode

	searchResponseBody, _ := ioutil.ReadAll(resp.Body)

	successResponse := api.SearchResponse{}
	if json.Unmarshal(searchResponseBody, &successResponse) == nil {
		searchResponse.Results = &successResponse
	} else {
		errorResponse := api.ErrorResponse{}
		if json.Unmarshal(searchResponseBody, &errorResponse) == nil {
			searchResponse.ErrorResponse = &errorResponse
		}
	}

	return searchResponse
}

func create(t *testing.T, queryString, contentType, content string) MetaResponse {
	var headers http.Header = nil
	if contentType != "" {
		headers = http.Header{}
		headers.Set("Content-Type", contentType)
	}

	resp, err := executeRequest("POST", fmt.Sprintf("/v1/blobs/data?%s", queryString), headers, strings.NewReader(content))
	require.Nil(t, err)

	return createMetaResponse(resp)
}

func read(t *testing.T, url string) ReadResponse {
	resp, err := executeRequest("GET", url, nil, nil)
	require.Nil(t, err)
	return populateBlobResponse(t, resp)
}

func get(t *testing.T, location string) MetaResponse {

	url, err := gourl.Parse(location)
	require.Nil(t, err)

	resp, err := executeRequest("GET", url.Path, nil, nil)
	require.Nil(t, err)

	return createMetaResponse(resp)
}

func createMetaResponse(resp *http.Response) MetaResponse {
	response := MetaResponse{}
	response.RawResponse = resp
	response.StatusCode = resp.StatusCode

	body, _ := ioutil.ReadAll(resp.Body)
	errorResponse := api.ErrorResponse{}
	if json.Unmarshal(body, &errorResponse) == nil {
		response.ErrorResponse = &errorResponse
	}

	goodStatusCodes := map[int]bool{http.StatusCreated: true, http.StatusOK: true}

	created := make(map[string]interface{})
	if json.Unmarshal(body, &created) == nil && goodStatusCodes[resp.StatusCode] {
		response.Meta = created
		response.Location = created["location"].(string)
		response.Data = created["data"].(string)
	}

	return response
}

func getLatestBlob(t *testing.T, queryString string) GetLatestResponse {
	resp, err := executeRequest("GET", fmt.Sprintf("/v1/blobs/data/latest?%s", queryString), nil, nil)
	require.Nil(t, err)
	return GetLatestResponse{
		ReadResponse: populateBlobResponse(t, resp),
		Location:     resp.Header.Get("Location"),
	}
}

func populateBlobResponse(t *testing.T, resp *http.Response) ReadResponse {
	readResponse := ReadResponse{}
	readResponse.Tags.CustomTags = make(map[string][]string)
	readResponse.RawResponse = resp
	readResponse.StatusCode = resp.StatusCode

	body, _ := ioutil.ReadAll(resp.Body)
	readResponse.Body = string(body)
	errorResponse := api.ErrorResponse{}
	if json.Unmarshal(body, &errorResponse) == nil {
		readResponse.ErrorResponse = &errorResponse
	}

	headers := resp.Header

	if subject, ok := headers[api.TagHeaderName("Subject")]; ok {
		assert.Len(t, subject, 1)
		readResponse.Subject = subject[0]
		delete(headers, "Subject")
	}

	if contentType, ok := headers["Content-Type"]; ok {
		assert.Len(t, contentType, 1)
		readResponse.Tags.ContentType = &contentType[0]
		delete(headers, "Content-Type")
	}

	if lastModified, ok := headers["Last-Modified"]; ok {
		assert.Len(t, lastModified, 1)
		t, _ := time.Parse(http.TimeFormat, lastModified[0])
		readResponse.CreatedAt = &t
		delete(headers, "Last-Modified")
	}

	if expiresAt, ok := headers["Expires"]; ok {
		assert.Len(t, expiresAt, 1)
		t, _ := time.Parse(http.TimeFormat, expiresAt[0])
		readResponse.ExpiresAt = &t
		delete(headers, "Expires")
	}

	reflectionTags := reflect.ValueOf(&readResponse.Tags).Elem()

	for k, v := range headers {
		if !strings.HasPrefix(k, api.TagHeaderPrefix) {
			continue
		}
		tagName := k[len(api.TagHeaderPrefix):]

		f := reflectionTags.FieldByName(tagName)
		if f.IsValid() {
			tagValue := v[0]
			if f.Kind() == reflect.Ptr {
				f.Set(reflect.ValueOf(&tagValue))
			} else {
				f.SetString(tagValue)
			}
		} else {
			readResponse.Tags.CustomTags[tagName] = v
		}
	}

	return readResponse
}

func executeRequest(method string, url string, headers http.Header, body io.Reader) (*http.Response, error) {
	if remoteUrl == nil {
		request := httptest.NewRequest(method, url, body)
		if headers != nil {
			request.Header = headers
		}
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, request)
		return resp.Result(), nil
	}

	parsedUrl, err := gourl.Parse(url)
	if err != nil {
		return nil, err
	}

	fullUrl := url

	if !parsedUrl.IsAbs() {
		parsedFullUrl := *remoteUrl
		parsedFullUrl.Path = path.Join(parsedFullUrl.Path, parsedUrl.Path)
		parsedFullUrl.RawQuery = parsedUrl.RawQuery
		fullUrl = parsedFullUrl.String()
	}

	request, err := http.NewRequest(method, fullUrl, body)
	if err != nil {
		return nil, err
	}

	if headers != nil {
		request.Header = headers
	}

	return http.DefaultClient.Do(request)
}

func TestGarbageCollection(t *testing.T) {
	if remoteUrl != nil {
		// this test only works in-proc
		return
	}

	keys := []core.BlobKey{
		createKey(t, "s1"),
		createKey(t, "s2"),
		createKey(t, "s3"),
	}

	for _, key := range keys {
		_, err := db.StageBlobMetadata(context.Background(), key, &core.BlobTags{})
		require.Nil(t, err)
		err = blobStore.SaveBlob(context.Background(), http.NoBody, key)
		require.Nil(t, err)
	}

	olderThan := time.Now().Add(time.Minute).UTC()

	err := core.CollectGarbage(context.Background(), db, blobStore, olderThan)
	require.Nil(t, err)

	for _, key := range keys {
		err := blobStore.ReadBlob(context.Background(), io.Discard, key)
		assert.ErrorIs(t, err, core.ErrBlobNotFound)
		assert.Nil(t, blobStore.DeleteBlob(context.Background(), key))
	}
}

func TestGarbageCollectionRemovesExpiredBlobs(t *testing.T) {

	if remoteUrl != nil {
		// this test only works in-proc
		return
	}

	ttl := "5m"
	body := "This is a body."
	session := "time-to-live-test"
	subject := fmt.Sprint(time.Now().UnixNano())

	createResponse := create(
		t,
		fmt.Sprintf("subject=%s&session=%s&_ttl=%s", subject, session, ttl),
		"text/plain",
		body)
	require.Equal(t, http.StatusCreated, createResponse.StatusCode)

	olderThan := time.Now().Add(time.Hour).UTC()
	err := core.CollectGarbage(context.Background(), db, blobStore, olderThan)
	require.Nil(t, err)

	// GC should have deleted the blob; search should now be empty.
	searchResponse := search(t, fmt.Sprintf("subject=%s&session=%s", subject, session))
	require.Equal(t, http.StatusOK, searchResponse.StatusCode)
	require.Empty(t, searchResponse.Results.Items)

	// Getting the blob directly should result in a 404
	metaResponse := get(t, createResponse.Location)
	require.Equal(t, http.StatusNotFound, metaResponse.StatusCode)

	dataResponse := read(t, createResponse.Data)
	require.Equal(t, http.StatusNotFound, dataResponse.StatusCode)
}

func TestStagedBlobsAreNotVisible(t *testing.T) {
	if remoteUrl != nil {
		// this test only works in-proc
		return
	}

	subject := fmt.Sprint(time.Now().UnixNano())

	key := createKey(t, subject)
	tags := core.BlobTags{CustomTags: make(map[string][]string)}

	_, err := db.StageBlobMetadata(context.Background(), key, &tags)
	require.Nil(t, err)
	err = blobStore.SaveBlob(context.Background(), http.NoBody, key)
	require.Nil(t, err)

	query := fmt.Sprintf("subject=%s", subject)

	searchResponse := search(t, query)
	assert.Empty(t, searchResponse.Results.Items)
	latestResponse := getLatestBlob(t, query)
	assert.Equal(t, http.StatusNotFound, latestResponse.StatusCode)

	err = db.CompleteStagedBlobMetadata(context.Background(), key)
	require.Nil(t, err)

	searchResponse = search(t, query)
	assert.Len(t, searchResponse.Results.Items, 1)
	latestResponse = getLatestBlob(t, query)
	assert.Equal(t, http.StatusOK, latestResponse.StatusCode)
}

func createKey(t *testing.T, subject string) core.BlobKey {
	id := uuid.New()
	return core.BlobKey{Subject: subject, Id: id}
}

type Response struct {
	StatusCode    int
	RawResponse   *http.Response
	ErrorResponse *api.ErrorResponse
}

type SearchResponse struct {
	Response
	Results *api.SearchResponse
}

type MetaResponse struct {
	Response
	Location string
	Data     string
	Meta     map[string]interface{}
}

type ReadResponse struct {
	Response
	CreatedAt *time.Time
	ExpiresAt *time.Time
	Body      string
	Subject   string
	Tags      core.BlobTags
}

type GetLatestResponse struct {
	ReadResponse
	Location string
}
