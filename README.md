# MRD Storage Server

The MRD Storage Server provides a simple RESTful API for storing and retrieving data during MRI image reconstructions. It supports:

- Creating a blob
- Reading a blob
- Searching for blobs
- Retrieving the latest blob matching a search expression


Blobs are stored with a set of metdata tags and searches are expressed as a set of filters over the tags. For creation and search, tags are specified using URI query string parameters. When retrieving a blob, the tags are included as HTTP reponse headers.

The tags are:

| Tag                | HTTP Header            | Query Parameter   | Comments                                                                                                                    |
|--------------------|------------------------|-------------------|-----------------------------------------------------------------------------------------------------------------------------|
| `subject`          | `mrd-tag-subject`      | `subject`         | The patient ID. Must be specified when creating and searching. Can be explictly `$null`.                                    |
| `device`           | `mrd-tag-device`       | `device`          | The device or scanner ID.                                                                                                   |
| `session`          | `mrd-tag-session`      | `session`         | The session ID.                                                                                                             |
| `name`             | `mrd-tag-name`         | `name`            | The name of the blob.                                                                                                       |
| `[custom-tag]`     | `mrd-tag-[custom-tag]` | `[custom-tag]`    | Custom tag names cannot collide with existing tags in this table. Unlike system tags, custom tags can have multiple values. |
| `content-type`     | `Content-Type`         | `N/A`             | The blob MIME type. Using the standard HTTP header.                                                                         |
| `location`         | `Location`             | `N/A`             | A URI for reading the blob metadata. System-assigned and globally unique. `[base]/v1/blobs/{{id}}`                                    |
| `data`             | `N/A`                  | `N/A`             | A URI for reading the blob data. System-assigned and globally unique. `[base]/v1/blobs/{{id}}/data`
| `last-modified`    | `Last-Modified`        | `N/A`             | The blob's creation timestamp. Using the standard HTTP header, even though blobs are immutable.                             |

Tag names are case-insensitive, but their values are case-sensitive.

## Example Interactions

### Creating a Blob

Tag values are specified as query string parameters of a `POST` request:

```
POST http://localhost:3333/v1/blobs/data?subject=123&session=mysession&name=NoiseCovariance
Content-Type: text/plain

This is my content
```

Response:
```
HTTP/1.1 201 Created
Date: Fri, 05 Nov 2021 10:51:54 GMT
Content-Length: 310
Content-Type: text/plain; charset=utf-8

{
   "contentType":"text/plain",
   "data":"http://localhost:3333/v1/blobs/c8a3aa43-04c0-4acb-9154-ce7b281ec274-123/data",
   "lastModified":"2021-11-05T11:51:54.036+01:00",
   "location":"http://localhost:3333/v1/blobs/c8a3aa43-04c0-4acb-9154-ce7b281ec274-123",
   "name":"NoiseCovariance",
   "session":"mysession",
   "subject":"123"
}
```

### Reading a Blob

The `data` attribute in the `POST` response body contains a link where you can `GET` the blob content:
```
GET http://localhost:3333/v1/blobs/c8a3aa43-04c0-4acb-9154-ce7b281ec274-123/data
```

Response:

```
HTTP/1.1 200 OK
Content-Type: text/plain
Last-Modified: Fri, 05 Nov 2021 11:51:54 GMT
Mrd-Tag-Name: NoiseCovariance
Mrd-Tag-Session: mysession
Mrd-Tag-Subject: 123
Date: Fri, 05 Nov 2021 10:54:30 GMT
Content-Length: 18

This is my content
```
Note that the tag values are added as HTTP headers with the prefix `Mrd-Tag-`.

### Searching Blobs:

You can search for blobs based on tags using the same syntax that is used for creating blobs:

```
GET http://localhost:3333/v1/blobs?subject=123&session=mysession&name=NoiseCovariance
```

Response:
```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Fri, 05 Nov 2021 10:55:26 GMT
Content-Length: 322

{
  "items": [
    {
      "contentType": "text/plain",
      "data": "http://localhost:3333/v1/blobs/c8a3aa43-04c0-4acb-9154-ce7b281ec274-123/data",
      "lastModified": "2021-11-05T11:51:54.036+01:00",
      "location": "http://localhost:3333/v1/blobs/c8a3aa43-04c0-4acb-9154-ce7b281ec274-123",
      "name": "NoiseCovariance",
      "session": "mysession",
      "subject": "123"
    }
  ]
}
```

Items are sorted in descending order of creation. If not all results fit in a single response, there will be a `nextLink` field in the response:

```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Fri, 05 Nov 2021 10:57:43 GMT
Content-Length: 458

{
  "items": [
    {
      "contentType": "text/plain",
      "data": "http://localhost:3333/v1/blobs/b8b1cac9-f9e6-4f86-b6c6-5eb6bd6cef2a-123/data",
      "lastModified": "2021-11-05T11:57:16.605+01:00",
      "location": "http://localhost:3333/v1/blobs/b8b1cac9-f9e6-4f86-b6c6-5eb6bd6cef2a-123",
      "name": "NoiseCovariance",
      "session": "mysession",
      "subject": "123"
    }
  ],
  "nextLink": "http://localhost:3333/v1/blobs?_ct=eyJ0cyI6MTYzNjEwOTgzNjYwNX0&_limit=1&name=NoiseCovariance&session=mysession&subject=123"
}
```

It is also possible to only get results that were created at or before a specific time with the `_at` parameter. The `_at` parameter is specified as a [time zone offset string](https://developer.mozilla.org/en-US/docs/Web/HTML/Date_and_time_formats#time_zone_offset_string). For example:

```
GET http://localhost:3333/v1/blobs?subject=123&session=mysession&name=NoiseCovariance&_at=2021-10-19T15:07:17.224Z
```

This will exclude results that were created after the given time.

### Getting the Latest Blob Matching a Query

There is a shortcut to get the latest blob matching a search query in one request at the `/v1/blobs/latest` endpoint:

```
GET http://localhost:3333/v1/blobs/data/latest?subject=123&session=mysession&name=NoiseCovariance
```

Response:
```
HTTP/1.1 200 OK
Content-Type: text/plain
Last-Modified: Fri, 05 Nov 2021 11:57:16 GMT
Location: http://localhost:3333/v1/blobs/b8b1cac9-f9e6-4f86-b6c6-5eb6bd6cef2a-123
Mrd-Tag-Name: NoiseCovariance
Mrd-Tag-Session: mysession
Mrd-Tag-Subject: 123
Date: Fri, 05 Nov 2021 11:03:58 GMT
Content-Length: 18

This is my content
```

The `_at` parameter can be used here as well to request the latest blob that was created no later than a given time.

```
GET http://localhost:3333/v1/blobs/data/latest?subject=123&session=mysession&name=NoiseCovariance&_at=2021-10-19T15:07:17.224Z
```

### Custom tags

Custom tags can be provded for blobs. Unlike system tags, custom tags can have many values:

```
POST http://localhost:3333/v1/blobs/data?subject=someone&customTag1=a&customTag1=b
```

```
HTTP/1.1 201 Created
Date: Fri, 05 Nov 2021 11:05:44 GMT
Content-Length: 298
Content-Type: text/plain; charset=utf-8

{"contentType":"text/plain","customtag1":["a","b"],"data":"http://localhost:3333/v1/blobs/35d4ae47-6e1c-4881-adb9-5581cededcbb-someone/data","lastModified":"2021-11-05T12:05:44.037+01:00","location":"http://localhost:3333/v1/blobs/35d4ae47-6e1c-4881-adb9-5581cededcbb-someone","subject":"someone"}
```

When a custom tag has multiple values, searches match any of the values:

```
GET http://localhost:3333/v1/blobs?subject=someone&customTag1=a
```

```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Fri, 05 Nov 2021 11:06:58 GMT
Content-Length: 310

{
  "items": [
    {
      "contentType": "text/plain",
      "customtag1": [
        "a",
        "b"
      ],
      "data": "http://localhost:3333/v1/blobs/35d4ae47-6e1c-4881-adb9-5581cededcbb-someone/data",
      "lastModified": "2021-11-05T12:05:44.037+01:00",
      "location": "http://localhost:3333/v1/blobs/35d4ae47-6e1c-4881-adb9-5581cededcbb-someone",
      "subject": "someone"
    }
  ]
}
```

However, when specifying multiple criteria on a tag, they must all match (they are ANDed togther, not ORed):

```
GET http://localhost:3333/v1/blobs?subject=someone&customTag1=a&customTag1=missing
```

```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Fri, 05 Nov 2021 11:07:38 GMT
Content-Length: 13

{
  "items": []
}
```

```
GET http://localhost:3333/v1/blobs?subject=someone&customTag1=a&customTag1=b
```

```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Fri, 05 Nov 2021 11:08:05 GMT
Content-Length: 310

{
  "items": [
    {
      "contentType": "text/plain",
      "customtag1": [
        "a",
        "b"
      ],
      "data": "http://localhost:3333/v1/blobs/35d4ae47-6e1c-4881-adb9-5581cededcbb-someone/data",
      "lastModified": "2021-11-05T12:05:44.037+01:00",
      "location": "http://localhost:3333/v1/blobs/35d4ae47-6e1c-4881-adb9-5581cededcbb-someone",
      "subject": "someone"
    }
  ]
}
```

## Data Store Providers

Blob Metadata (tags) are stored separately from the blob contents. We currently support [PostgreSQL](https://www.postgresql.org/) and [SQLite](https://www.sqlite.org/) for the metadata and the filesystem or [Azure Blob Storage](https://azure.microsoft.com/en-us/services/storage/blobs/) for storing blob contents.

## Getting Started

By default, the storage server uses SQLite and the filesystem. The behavior of the server can be configured using environment variables:

| Variable                                      | Type    | Description                                                                                                              | Default Value       |
|-----------------------------------------------|---------|--------------------------------------------------------------------------------------------------------------------------|---------------------|
| MRD_STORAGE_SERVER_DATABASE_PROVIDER          | string  | The metadata database provider. Can be `sqlite` or `postgresql`.                                                         | sqlite              |
| MRD_STORAGE_SERVER_DATABASE_CONNECTION_STRING | string  | The provider-specific connection string. For SQLite, the path to the database file.                                      | ./_data/metadata.db |
| MRD_STORAGE_SERVER_STORAGE_PROVIDER           | string  | The blob storage provider. Can be `filesystem` or `azureblob`.                                                           | filesystem          |
| MRD_STORAGE_SERVER_STORAGE_CONNECTION_STRING  | string  | The provider-specific connection string. For the filesystem provider, the path to the directory in which to store files. | ./_data/blobs       |
| MRD_STORAGE_SERVER_STORAGE_PORT               | integer | The port to listen on.                                                                                                   | 3333                |
| MRD_STORAGE_SERVER_STORAGE_LOG_REQUESTS       | boolean | Whether to log the URI, status code, and duration of each HTTP request.                                                  | true                |

## TODO:

- Handle secrets as files
- Migration tool
- Support Azure Managed identity
- Support Delete
- Support TTL on blobs
- Swagger
- Health check
- Support TLS
- Publish releases with binaries for each supported platform
