#!/usr/bin/env bash

set -euo pipefail

EXTERNAL_REPO="${EXTERNAL_REPO:-1345695/Xray-core}"
REMOTE_URL="https://github.com/${EXTERNAL_REPO}.git"

latest_head_sha="$(git ls-remote --heads "$REMOTE_URL" main | awk '{print $1}')"
if [[ -z "$latest_head_sha" ]]; then
  echo "Failed to resolve latest commit from ${EXTERNAL_REPO} main" >&2
  exit 1
fi

go_version="$(curl -fsSL "https://raw.githubusercontent.com/${EXTERNAL_REPO}/${latest_head_sha}/go.mod" | awk '/^go / {print $2; exit}')"
if [[ -z "$go_version" ]]; then
  echo "Failed to detect Go version from upstream go.mod" >&2
  exit 1
fi

current_replace_version="$(
  awk -v repo="$EXTERNAL_REPO" '
    $1 == "replace" &&
    $2 == "github.com/xtls/xray-core" &&
    $3 == "=>" &&
    $4 == "github.com/" repo {
      print $5
      exit
    }
  ' go.mod
)"
if [[ -z "$current_replace_version" ]]; then
  echo "Failed to detect current replace target for github.com/xtls/xray-core" >&2
  exit 1
fi

latest_head_short_sha="${latest_head_sha:0:12}"
current_replace_short_sha="${current_replace_version##*-}"
current_replace_short_sha="${current_replace_short_sha:0:12}"
release_tag="xray-core-${latest_head_short_sha}"

if [[ -n "${GITHUB_ENV:-}" ]]; then
  {
    echo "LATEST_HEAD_SHA=${latest_head_sha}"
    echo "LATEST_HEAD_SHORT_SHA=${latest_head_short_sha}"
    echo "CURRENT_REPLACE_VERSION=${current_replace_version}"
    echo "CURRENT_REPLACE_SHORT_SHA=${current_replace_short_sha}"
    echo "GO_VERSION=${go_version}"
    echo "RELEASE_TAG=${release_tag}"
  } >> "$GITHUB_ENV"
fi

if [[ "$current_replace_short_sha" == "$latest_head_short_sha" ]]; then
  echo "Pinned xray-core already matches upstream main (${latest_head_short_sha})"
  if [[ -n "${GITHUB_ENV:-}" ]]; then
    echo "UPDATED=false" >> "$GITHUB_ENV"
  fi
  exit 0
fi

echo "Updating xray-core from ${current_replace_version} to main (${latest_head_short_sha})"
echo "Syncing Go version to ${go_version}"

go mod edit -go="$go_version"
go mod edit "-replace=github.com/xtls/xray-core=github.com/${EXTERNAL_REPO}@${latest_head_sha}"
go mod tidy -v

if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "UPDATED=true" >> "$GITHUB_ENV"
fi

git diff -- go.mod go.sum
