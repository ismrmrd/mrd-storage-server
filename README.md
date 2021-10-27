# MRD Storage Server

This repo contains a server with a RESTful API for storing and retrieving data during MRI image reconstructions. It supports storing and arbitrary blobs with a set of metadata tags and searching them using these tags.

The tags are:

| Tag                | HTTP Header            | Query Parameter   | Comments                                                                                                                    |
|--------------------|------------------------|-------------------|-----------------------------------------------------------------------------------------------------------------------------|
| `subject`          | `mrd-tag-subject`      | `subject`         | Must be specified when creating  and searching. Can be explictly `$null`                                                    |
| `device`           | `mrd-tag-device`       | `device`          |                                                                                                                             |
| `session`          | `mrd-tag-session`      | `session`         |                                                                                                                             |
| `name`             | `mrd-tag-name`         | `name`            |                                                                                                                             |
| `[custom-tag]`     | `mrd-tag-[custom-tag]` | `[custom-tag]`    | Custom tag names cannot collide with existing tags in this table. Unlike system tags, custom tags can have multiple values. |
| `content-type`     | `Content-Type`         | `N/A`             | Using standard HTTP header.                                                                                                 |
| `uri`              | `Location`             | `N/A`             | System-assigned and globally unique. `[base]/v1/blob/{{id}}`                                                                |
| `last-modified`    | `Last-Modified`        | `N/A`             | Using standard HTTP header, even though blobs are immutable.                                                                |

Tag names are case-insensitive, but their values are case-sensitive.

## Creating a Blob

Tag values are specified as query string parameters of a `POST` request:

```
POST http://localhost:3333/v1/blob?subject=123&session=mysession&name=NoiseCovariance
Content-Type: text/plain

This is my content
```

Response:
```
HTTP/1.1 201 Created
Location: http://localhost:3333/v1/blob/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123
Date: Wed, 20 Oct 2021 15:07:17 GMT
Content-Length: 0
Connection: close
```

## Reading a Blob

The `Location` header of the `POST` response contains a link the `GET` the blob:
```
GET http://localhost:3333/v1/blob/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123
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

## Searching Blobs:

You can search for blobs based on tags using the same syntax that is used for creating blobs:

```
GET http://localhost:3333/v1/blob?subject=123&session=mysession&name=NoiseCovariance
```

Response:
``` json
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
      "location": "http://localhost:3333/v1/blob/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123",
      "name": "NoiseCovariance",
      "session": "mysession",
      "subject": "123"
    }
  ]
}
```

Items are sorted in descing order of creation. If not all results fit in a single reponse, there will be a `nextLink` field in the response:
``` json
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
      "location": "http://localhost:3333/v1/blob/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123",
      "name": "NoiseCovariance",
      "session": "mysession",
      "subject": "123"
    }
  ],
  "nextLink": "http://localhost:3333/v1/blob?_ct=eyJ0cyI6MTYzNDc0MjQzNzIyNH0&_limit=1&name=NoiseCovariance&session=mysession&subject=123"
}
```

## Getting the Latest Blob Matching a Query

There is a shortcut to get the latest blob matching a search query in one request at the `/v1/blob/latest` endpoint:

```
GET http://localhost:3333/v1/blob/latest?subject=123&session=mysession&name=NoiseCovariance
```

Response:
```
HTTP/1.1 200 OK
Content-Type: text/plain
Last-Modified: Wed, 20 Oct 2021 15:07:17 GMT
Location: http://localhost:3333/v1/blob/49faf89d-9e4d-4ff5-9b2c-d1db39bcdb2b-123
Mrd-Tag-Name: NoiseCovariance
Mrd-Tag-Session: mysession
Mrd-Tag-Subject: 123
Date: Wed, 20 Oct 2021 15:15:41 GMT
Content-Length: 18
Connection: close

This is my content
```

## Custom tags

Custom tags can be provded for blobs. Unlike system tags, custom tags can have many values:

```
POST http://localhost:3333/v1/blob?subject=someone&customTag1=a&customTag1=b
```

```
HTTP/1.1 201 Created
Location: http://localhost:3333/v1/blob/9e5c32e1-4b76-4098-bf72-48602efc274f-someone
Date: Thu, 21 Oct 2021 21:01:53 GMT
Content-Length: 0
Connection: close
```

When a custom tag has multiple values, searches match any of the values:

```
GET http://localhost:3333/v1/blob?subject=someone&customTag1=a
```

``` json
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
      "location": "http://localhost:3333/v1/blob/9e5c32e1-4b76-4098-bf72-48602efc274f-someone",
      "subject": "someone"
    }
  ]
}
```

However, when specifying multiple criteria on a tag, they must all match (they are ANDed togther, not ORed):

```
GET http://localhost:3333/v1/blob?subject=someone&customTag1=a&customTag1=missing
```

``` json
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
GET http://localhost:3333/v1/blob?subject=someone&customTag1=a&customTag1=b
```

``` json
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
      "location": "http://localhost:3333/v1/blob/9e5c32e1-4b76-4098-bf72-48602efc274f-someone",
      "subject": "someone"
    }
  ]
}
```

## TODO:

- Handle secrets as files
- Migration tool
- Support Managed identity
- Support Delete
- Support TTL on blobs
- Swagger
- Health check
