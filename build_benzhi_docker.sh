#!/usr/bin/env bash
# Build the benzhi (评测) Docker image for the thin-section mineral boundary review service.
# Usage: bash build_benzhi_docker.sh <image-name> <platform>
#   platform example: linux/amd64 or linux/arm64
set -euo pipefail

IMAGE_NAME="${1:-my-project}"
PLATFORM="${2:-linux/amd64}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Building $IMAGE_NAME for $PLATFORM using benzhi.Dockerfile"
docker buildx build --platform "$PLATFORM" -f benzhi.Dockerfile -t "$IMAGE_NAME" .
echo "Done: $IMAGE_NAME @ $PLATFORM"
