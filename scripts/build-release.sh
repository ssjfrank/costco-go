#!/usr/bin/env bash
# Builds costco-cli for every published platform into an output directory,
# with a SHA256SUMS file. Used by .github/workflows/release.yml.
#
#   scripts/build-release.sh [output-dir]
set -euo pipefail

out="${1:-dist}"
commit="$(git rev-parse --short HEAD)"

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
  name="costco-cli-${goos}-${goarch}"
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

(cd "$out" && sha256sum costco-cli-* > SHA256SUMS)
echo "wrote ${out}/SHA256SUMS"
