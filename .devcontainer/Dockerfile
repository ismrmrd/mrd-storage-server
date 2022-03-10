# See here for image contents: https://github.com/microsoft/vscode-dev-containers/tree/v0.202.3/containers/go/.devcontainer/base.Dockerfile

# [Choice] Go version (use -bullseye variants on local arm64/Apple Silicon): 1, 1.16, 1.17, 1-bullseye, 1.16-bullseye, 1.17-bullseye, 1-buster, 1.16-buster, 1.17-buster
ARG VARIANT="1.17-bullseye"
FROM mcr.microsoft.com/vscode/devcontainers/go:0-${VARIANT}

RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends sqlite3

ARG MOCKGEN_VERSION="v1.6.0"
RUN go install github.com/golang/mock/mockgen@${MOCKGEN_VERSION}

RUN chmod -R g+w /go
