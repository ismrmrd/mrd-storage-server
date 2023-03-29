#!/bin/bash

set -euo pipefail

echo "env..."
env

echo "which go..."
which go

go build -o "${PREFIX}/bin/mrd-storage-server" .
