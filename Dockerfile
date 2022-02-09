# Start by building the application.
FROM golang:1.17-bullseye as build

# create an empty directory that we will use as a COPY source from the final stage
# so that the nonroot owns the /data directory (there is no mkdir in distroless)
WORKDIR /empty

# create files with default connections strings for the runtime image
WORKDIR /defaults
RUN echo "/data/metadata.db" > /defaults/database_connection_string \
    && echo "/data/blobs" > /defaults/storage_connection_string

WORKDIR /go/src/app

COPY ./go.mod .

RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build go build -o /go/bin/app

# Now copy it into our base image.
FROM gcr.io/distroless/base-debian11

# Set up /data as the default storage directory for filesystem-based providers.
COPY --from=build --chown=nonroot:nonroot /empty/ /data

# Set up defaults for the database and storage connections strings.
# Here we use the _FILE suffix for the environment variables, which can be overridden
# by setting the variable to the path of a mounted secret. This is often more secure than
# providing connection string directly in the variables without the _FILE suffix, but either
# approach will override the defaults set here.
COPY --from=build --chown=nonroot:nonroot /defaults/ /defaults/
ENV MRD_STORAGE_SERVER_DATABASE_CONNECTION_STRING_FILE=/defaults/database_connection_string
ENV MRD_STORAGE_SERVER_STORAGE_CONNECTION_STRING_FILE=/defaults/storage_connection_string

COPY --from=build /go/bin/app /

USER nonroot:nonroot
ENTRYPOINT ["/app"]
