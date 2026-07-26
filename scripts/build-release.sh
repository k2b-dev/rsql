#!/usr/bin/env bash

set -euo pipefail

tag="${1:?usage: build-release.sh <vX.Y.Z> [output-directory]}"
output_dir="${2:-release-dist}"

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid release tag: $tag" >&2
  exit 1
fi

if [[ -e "$output_dir" ]]; then
  echo "output path already exists: $output_dir" >&2
  exit 1
fi

version="${tag#v}"
cli_version="$(go run ./cmd/rsql version)"
package_version="$(sed -n 's/^[[:space:]]*"version": "\([^"]*\)",/\1/p' client/package.json | head -n 1)"

if [[ "$cli_version" != "rsql v$version" ]]; then
  echo "CLI version does not match $tag: $cli_version" >&2
  exit 1
fi

if [[ "$package_version" != "$version" ]]; then
  echo "npm package version does not match $tag: $package_version" >&2
  exit 1
fi

mkdir -p "$output_dir"
work_dir="$(mktemp -d)"

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  goos="${target%/*}"
  goarch="${target#*/}"
  name="rsql_${version}_${goos}_${goarch}"
  staging="$work_dir/$name"

  mkdir -p "$staging"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
    -trimpath \
    -ldflags "-s -w" \
    -o "$staging/rsql" \
    ./cmd/rsql
  cp LICENSE README.md "$staging/"
  tar -C "$staging" -czf "$output_dir/$name.tar.gz" .
done

(
  cd "$output_dir"
  shasum -a 256 ./*.tar.gz > checksums.txt
)
