#!/usr/bin/env bash

set -euo pipefail

tag="${1:?usage: build-release.sh <vX.Y.Z> [output-directory]}"
output_dir="${2:-release-dist}"
nfpm_bin="${NFPM_BIN:-nfpm}"

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid release tag: $tag" >&2
  exit 1
fi

if ! command -v "$nfpm_bin" >/dev/null 2>&1; then
  echo "nFPM is required to build Linux packages: $nfpm_bin" >&2
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
trap 'rm -rf "$work_dir"' EXIT

host_goos="$(go env GOOS)"
host_goarch="$(go env GOARCH)"
source_date_epoch="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct)}"
export SOURCE_DATE_EPOCH="$source_date_epoch"

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  goos="${target%/*}"
  goarch="${target#*/}"
  name="rsql_${version}_${goos}_${goarch}"
  staging="$work_dir/$name"
  binary="$work_dir/bin/$goos/$goarch/rsql"

  mkdir -p "$staging" "$(dirname "$binary")"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
    -trimpath \
    -ldflags "-s -w" \
    -o "$binary" \
    ./cmd/rsql
  cp "$binary" "$staging/rsql"
  cp LICENSE README.md "$staging/"
  tar -C "$staging" -czf "$output_dir/$name.tar.gz" .

  if [[ "$goos" == "$host_goos" && "$goarch" == "$host_goarch" ]]; then
    actual_version="$("$binary" version)"
    if [[ "$actual_version" != "rsql v$version" ]]; then
      echo "built binary version does not match $tag: $actual_version" >&2
      exit 1
    fi
  fi

  if [[ "$goos" == "linux" ]]; then
    case "$goarch" in
      amd64) rpm_arch="x86_64" ;;
      arm64) rpm_arch="aarch64" ;;
      *)
        echo "unsupported package architecture: $goarch" >&2
        exit 1
        ;;
    esac

    export RSQL_PACKAGE_ARCH="$goarch"
    export RSQL_PACKAGE_BINARY="$binary"
    export RSQL_PACKAGE_VERSION="$version"

    "$nfpm_bin" package \
      --config packaging/nfpm.yaml \
      --packager deb \
      --target "$output_dir/rsql_${version}_${goarch}.deb"
    "$nfpm_bin" package \
      --config packaging/nfpm.yaml \
      --packager rpm \
      --target "$output_dir/rsql-${version}-1.${rpm_arch}.rpm"
  fi
done

(
  cd "$output_dir"
  shasum -a 256 ./*.tar.gz ./*.deb ./*.rpm > checksums.txt
)
