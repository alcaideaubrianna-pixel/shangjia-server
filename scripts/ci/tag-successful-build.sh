#!/usr/bin/env bash
# Called only after the image has been built and pushed successfully.
set -euo pipefail

: "${BUILD_VERSION:?build version is required}"
: "${GITHUB_SHA:?built commit is required}"
if [[ ! "$BUILD_VERSION" =~ ^[0-9]+\.[0-9]+$ ]]; then
  echo "Invalid build version: $BUILD_VERSION" >&2
  exit 1
fi
git cat-file -e "${GITHUB_SHA}^{commit}"

tag_ref="refs/tags/${BUILD_VERSION}"
existing_sha=$(git ls-remote --exit-code --refs origin "$tag_ref") || {
  result=$?
  # ls-remote returns 2 only when no matching ref exists.
  if [ "$result" -ne 2 ]; then
    exit "$result"
  fi
}
if [ -n "$existing_sha" ]; then
  if [ "${existing_sha%%$'\t'*}" != "$GITHUB_SHA" ]; then
    echo "Build tag already points to another commit: $BUILD_VERSION" >&2
    exit 1
  fi
  echo "Build tag already exists: $BUILD_VERSION"
  exit 0
fi

# Push the exact built commit; never tag the potentially newer branch head.
git push origin "${GITHUB_SHA}:${tag_ref}"
