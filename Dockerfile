# Start by building the application.
FROM golang:1.17-bullseye as build

# create an empty directory that we will use as a COPY source from the final stage
# so that the nonroot owns the /data directory (there is no mkdir in distroless)
WORKDIR /empty

WORKDIR /go/src/app

COPY ./go.mod .

RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build go build -o /go/bin/app

# Now copy it into our base image.
FROM gcr.io/distroless/base-debian11

# Set up /data as the default storage directory for filesystem-based providers.
COPY --from=build --chown=nonroot:nonroot /empty/ /data
ENV MRD_STORAGE_SERVER_DATABASE_CONNECTION_STRING=/data/metadata.db
ENV MRD_STORAGE_SERVER_STORAGE_CONNECTION_STRING=/data/blobs

COPY --from=build /go/bin/app /

USER nonroot:nonroot
ENTRYPOINT ["/app"]
