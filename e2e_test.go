package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	urlPkg "net/url"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ismrmrd/mrd-storage-api/api"
	"github.com/ismrmrd/mrd-storage-api/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var router http.Handler
var remoteUrl *urlPkg.URL

func init() {

	if remoteUrlVar := os.Getenv("TEST_REMOTE_URL"); remoteUrlVar != "" {
		var err error
		remoteUrl, err = url.Parse(remoteUrlVar)
		if err != nil {
			log.Fatalf("Invalid TEST_REMOTE_URL value")
		}

		return
	}

	config := loadConfig()
	config.LogRequests = false

	dbProvider := os.Getenv("TEST_DB_PROVIDER")

	switch dbProvider {
	case ConfigDatabaseProviderPostgresql:
		config.DatabaseProvider = ConfigDatabaseProviderPostgresql
		config.DatabaseConnectionString = "user=mrd password=mrd dbname=mrd host=localhost port=9920 sslmode=disable"
	case "", ConfigDatabaseProviderSqlite:
		// use defaults
	default:
		log.Fatalf("Unrecognized TEST_DB_PROVIDER environment variable '%s'", dbProvider)
	}

	storageProvider := os.Getenv("TEST_STORAGE_PROVIDER")

	switch storageProvider {
	case ConfigStorageProviderAzureBlob:
		config.StorageProvider = ConfigStorageProviderAzureBlob
		config.StorageConnectionString = "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://localhost:10000/devstoreaccount1;"
	case "", ConfigStorageProviderFileSystem:
		// use defaults
	default:
		log.Fatalf("Unrecognized TEST_STORAGE_PROVIDER environment variable '%s'", storageProvider)
	}

	_router, err := assembleHandler(config)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	router = _router
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
	assert.Equal(t, http.StatusCreated, createResp.StatusCode)

	// now read the blob using the Location header in the response
	location := createResp.Location

	readResp := read(t, location)
	assert.Equal(t, http.StatusOK, readResp.StatusCode)
	assert.Equal(t, bodyContents, readResp.Body)
	assert.Equal(t, "application/octet-stream", *readResp.Tags.ContentType)
	assert.NotNil(t, readResp.CreatedAt)
	assert.Equal(t, subject, readResp.Tags.Subject)
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
	assert.Equal(t, http.StatusCreated, createResp.StatusCode)

	// now read the blob using the Location header in the response
	location := createResp.Location

	readResp := read(t, location)
	assert.Equal(t, http.StatusOK, readResp.StatusCode)
	assert.Equal(t, bodyContents, readResp.Body)

	assert.Equal(t, "text/plain", *readResp.Tags.ContentType)
	assert.NotNil(t, readResp.CreatedAt)
	assert.Equal(t, subject, readResp.Tags.Subject)
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

			link := fmt.Sprintf("/blob?subject=%s&mytag=t&_limit=%d", subject, pageSize)

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
			atQuery := fmt.Sprintf("%s&_at=%s", originalQuery, thisTime)
			atRes := search(t, atQuery)
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

func Test404(t *testing.T) {
	cases := []string{
		"/",
		fmt.Sprintf("/blob/latest?subject=%d", time.Now().UnixNano()),
		"/blob/abc",
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
	readResp := read(t, createResp.Location)
	assert.Equal(t, "hello", readResp.Body)

	latestResp := getLatestBlob(t, query)
	assert.Equal(t, "hello", latestResp.Body)
}

func search(t *testing.T, queryString string) SearchResponse {
	resp, err := executeRequest("GET", fmt.Sprintf("/blob?%s", queryString), nil, nil)
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

func create(t *testing.T, queryString, contentType, content string) CreateResponse {
	var headers http.Header = nil
	if contentType != "" {
		headers = http.Header{}
		headers.Set("Content-Type", contentType)
	}

	resp, err := executeRequest("POST", fmt.Sprintf("/blob?%s", queryString), headers, strings.NewReader(content))
	require.Nil(t, err)

	createResponse := CreateResponse{}
	createResponse.RawResponse = resp
	createResponse.StatusCode = resp.StatusCode

	body, _ := ioutil.ReadAll(resp.Body)
	errorResponse := api.ErrorResponse{}
	if json.Unmarshal(body, &errorResponse) == nil {
		createResponse.ErrorResponse = &errorResponse
	}

	createResponse.Location = resp.Header.Get("Location")

	return createResponse
}

func read(t *testing.T, url string) ReadResponse {
	resp, err := executeRequest("GET", url, nil, nil)
	require.Nil(t, err)
	return populateBlobResponse(resp)
}

func getLatestBlob(t *testing.T, queryString string) GetLatestResponse {
	resp, err := executeRequest("GET", fmt.Sprintf("/blob/latest?%s", queryString), nil, nil)
	require.Nil(t, err)
	return GetLatestResponse{
		ReadResponse: populateBlobResponse(resp),
		Location:     resp.Header.Get("Location"),
	}
}

func populateBlobResponse(resp *http.Response) ReadResponse {
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

	if contentType, ok := headers["Content-Type"]; ok {
		readResponse.Tags.ContentType = &contentType[0]
		delete(headers, "Content-Type")
	}

	if lastModified, ok := headers["Last-Modified"]; ok {
		t, _ := time.Parse(http.TimeFormat, lastModified[0])
		readResponse.CreatedAt = &t
		delete(headers, "Last-Modified")
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

	parsedUrl, err := urlPkg.Parse(url)
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

type Response struct {
	StatusCode    int
	RawResponse   *http.Response
	ErrorResponse *api.ErrorResponse
}

type SearchResponse struct {
	Response
	Results *api.SearchResponse
}

type CreateResponse struct {
	Response
	Location string
}

type ReadResponse struct {
	Response
	CreatedAt *time.Time
	Body      string
	Tags      core.BlobTags
}

type GetLatestResponse struct {
	ReadResponse
	Location string
}
