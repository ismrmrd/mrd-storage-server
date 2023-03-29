#!/bin/bash

set -euo pipefail

echo "build.sh: env..."
env

echo "build.sh: which go..."
which go

echo "build.sh: go build..."
go build -v -o "${PREFIX}/bin/mrd-storage-server" .
