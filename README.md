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
| `uri`              | `Location`             | `N/A`             | A URI for reading the blob. System-assigned and globally unique. `[base]/v1/blobs/{{id}}`                                    |
| `last-modified`    | `Last-Modified`        | `N/A`             | The blob's creation timestamp. Using the standard HTTP header, even though blobs are immutable.                             |

Tag names are case-insensitive, but their values are case-sensitive.

## Example Interactions

### Creating a Blob

Tag values are specified as query string parameters of a `POST` request:

```
POST http://localhost:3333/v1/blobs?subject=123&session=mysession&name=NoiseCovariance
Content-Type: text/plain

This is my content
```

Response:
```
HTTP/1.1 201 Created
Location: http://localhost:3333/v1/blobs/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123
Date: Wed, 20 Oct 2021 15:07:17 GMT
Content-Length: 0
Connection: close
```

### Reading a Blob

The `Location` header of the `POST` response contains a link the `GET` the blob:
```
GET http://localhost:3333/v1/blobs/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123
```

Response:

```
HTTP/1.1 200 OK
Content-Type: text/plain
Last-Modified: Wed, 20 Oct 2021 15:07:17 GMT
Mrd-Tag-Name: NoiseCovariance
Mrd-Tag-Session: mysession
Mrd-Tag-Subject: 123
Date: Wed, 20 Oct 2021 15:08:19 GMT
Content-Length: 18
Connection: close

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
Date: Wed, 20 Oct 2021 15:12:56 GMT
Content-Length: 359
Connection: close

{
  "items": [
    {
      "contentType": "text/plain",
      "lastModified": "2021-10-20T15:07:17.224Z",
      "location": "http://localhost:3333/v1/blobs/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123",
      "name": "NoiseCovariance",
      "session": "mysession",
      "subject": "123"
    }
  ]
}
```

Items are sorted in descing order of creation. If not all results fit in a single reponse, there will be a `nextLink` field in the response:

```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Wed, 20 Oct 2021 15:12:56 GMT
Content-Length: 359
Connection: close

{
  "items": [
    {
      "contentType": "text/plain",
      "lastModified": "2021-10-20T15:07:17.224Z",
      "location": "http://localhost:3333/v1/blobs/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123",
      "name": "NoiseCovariance",
      "session": "mysession",
      "subject": "123"
    }
  ],
  "nextLink": "http://localhost:3333/v1/blobs?_ct=eyJ0cyI6MTYzNDc0MjQzNzIyNH0&_limit=1&name=NoiseCovariance&session=mysession&subject=123"
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
GET http://localhost:3333/v1/blobs/latest?subject=123&session=mysession&name=NoiseCovariance
```

Response:
```
HTTP/1.1 200 OK
Content-Type: text/plain
Last-Modified: Wed, 20 Oct 2021 15:07:17 GMT
Location: http://localhost:3333/v1/blobs/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123
Mrd-Tag-Name: NoiseCovariance
Mrd-Tag-Session: mysession
Mrd-Tag-Subject: 123
Date: Wed, 20 Oct 2021 15:15:41 GMT
Content-Length: 18
Connection: close

This is my content
```

The `_at` parameter can be used here as well to request the latest blob that was created no later than a given time.

```
GET http://localhost:3333/v1/blobs/latest?subject=123&session=mysession&name=NoiseCovariance&_at=2021-10-19T15:07:17.224Z
```

### Custom tags

Custom tags can be provded for blobs. Unlike system tags, custom tags can have many values:

```
POST http://localhost:3333/v1/blobs?subject=someone&customTag1=a&customTag1=b
```

```
HTTP/1.1 201 Created
Location: http://localhost:3333/v1/blobs/9e5c32e1-4b76-4098-bf72-48602efc274f-someone
Date: Thu, 21 Oct 2021 21:01:53 GMT
Content-Length: 0
Connection: close
```

When a custom tag has multiple values, searches match any of the values:

```
GET http://localhost:3333/v1/blobs?subject=someone&customTag1=a
```

```
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Thu, 21 Oct 2021 21:05:16 GMT
Content-Length: 184
Connection: close

{
  "items": [
    {
      "customtag1": [
        "a",
        "b"
      ],
      "lastModified": "2021-10-21T21:01:53.666Z",
      "location": "http://localhost:3333/v1/blobs/9e5c32e1-4b76-4098-bf72-48602efc274f-someone",
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
Date: Thu, 21 Oct 2021 21:07:10 GMT
Content-Length: 13
Connection: close

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
Date: Thu, 21 Oct 2021 21:08:13 GMT
Content-Length: 184
Connection: close

{
  "items": [
    {
      "customtag1": [
        "a",
        "b"
      ],
      "lastModified": "2021-10-21T21:01:53.666Z",
      "location": "http://localhost:3333/v1/blobs/9e5c32e1-4b76-4098-bf72-48602efc274f-someone",
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
