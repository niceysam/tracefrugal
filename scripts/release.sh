#!/usr/bin/env bash
set -euo pipefail

release_version="${1:?usage: bash scripts/release.sh v0.1.0}"
if [[ ! "$release_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Expected a stable semantic version such as v0.1.0" >&2
  exit 2
fi
if [[ -e dist ]]; then
  echo "dist already exists; use a clean checkout to avoid stale release assets" >&2
  exit 2
fi
mkdir dist
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  target_os="${target%/*}"
  target_arch="${target#*/}"
  archive_name="tracefrugal_${release_version#v}_${target_os}_${target_arch}"
  staging_dir="$(mktemp -d)"
  binary_name=tracefrugal
  if [[ "$target_os" == windows ]]; then
    binary_name=tracefrugal.exe
  fi
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath \
    -ldflags "-s -w -X main.version=$release_version" \
    -o "$staging_dir/$binary_name" ./cmd/tracefrugal
  cp LICENSE README.md "$staging_dir/"
  if [[ "$target_os" == windows ]]; then
    archive_path="$PWD/dist/$archive_name.zip"
    (cd "$staging_dir" && zip -q "$archive_path" "$binary_name" LICENSE README.md)
  else
    tar -czf "dist/$archive_name.tar.gz" -C "$staging_dir" "$binary_name" LICENSE README.md
  fi
  rm -r "$staging_dir"
done
(cd dist && shasum -a 256 ./*.tar.gz ./*.zip > checksums.txt)
