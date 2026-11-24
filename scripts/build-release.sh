#!/usr/bin/env bash
# Builds costco-cli for every published platform into an output directory,
# with a checksum file. Used by .github/workflows/ci.yml and release.yml.
# Every file name carries the version from pkg/costco/constants.go, for example
# costco-cli-1.1.0-windows-amd64.exe and costco-cli-1.1.0-SHA256SUMS.txt.
#
#   scripts/build-release.sh [output-dir]
set -euo pipefail

out="${1:-dist}"
commit="$(git rev-parse --short HEAD)"
version="$(sed -n 's/^[[:space:]]*Version = "\([^"]*\)".*/\1/p' pkg/costco/constants.go)"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "pkg/costco/constants.go has no usable Version (found '${version}')" >&2
  exit 1
fi

targets=(
  "darwin arm64"   # Apple Silicon Macs (M1 and later)
  "darwin amd64"   # Intel Macs
  "linux amd64"
  "linux arm64"
  "windows amd64"
)

rm -rf "$out"
mkdir -p "$out"

for target in "${targets[@]}"; do
  read -r goos goarch <<<"$target"
  name="costco-cli-${version}-${goos}-${goarch}"
  [[ "$goos" == "windows" ]] && name="${name}.exe"

  # CGO off keeps the binaries self-contained. Go's own linker also ad-hoc
  # signs darwin/arm64 binaries, which Apple Silicon requires before it will
  # run them, so no macOS machine is needed to build.
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
    -trimpath \
    -ldflags "-s -w -X main.commit=${commit}" \
    -o "${out}/${name}" \
    ./cmd/costco-cli
  echo "built ${out}/${name}"
done

sums="costco-cli-${version}-SHA256SUMS.txt"
binaries=("$out"/costco-cli-*)
(cd "$out" && sha256sum "${binaries[@]##*/}" > "$sums")
echo "wrote ${out}/${sums}"
