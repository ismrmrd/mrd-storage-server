# Start by building the application.
FROM golang:1.17-bullseye as build

# create an empty directory that we will use as a COPY source from the final stage
# so that the nonroot owns the /data directory (there is no mkdir in distroless)
WORKDIR /empty

WORKDIR /go/src/app

COPY ./go.mod .

RUN go mod download

COPY . .

RUN go build -o /go/bin/app

# Now copy it into our base image.
FROM gcr.io/distroless/base-debian11

COPY --from=build --chown=nonroot:nonroot /empty/ /data
COPY --from=build /go/bin/app /

USER nonroot:nonroot
ENTRYPOINT ["/app"]
