#!/usr/bin/env bash
# Build Tandem as a single binary: web bundle → embedded → Go cross-compile.
#
#   scripts/build.sh            # host platform only → dist/tandem
#   scripts/build.sh --all      # every release target → dist/tandem_<os>_<arch>
#   scripts/build.sh --skip-web # reuse the existing apps/web/dist
#
# Output lands in ./dist. --all also writes .tar.gz/.zip archives and
# SHA256SUMS, which is exactly what .github/workflows/release.yml uploads.
# CGO is off throughout: the SQLite driver (modernc.org/sqlite) is pure Go.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ALL=0
SKIP_WEB=0
for arg in "$@"; do
  case "$arg" in
    --all) ALL=1 ;;
    --skip-web) SKIP_WEB=1 ;;
    -h|--help) sed -n 2,11p "$0"; exit 0 ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

VERSION="$(tr -d '[:space:]' < apps/api/VERSION)"
BUNDLE="apps/api/internal/webdist/bundle"

if [[ "$SKIP_WEB" == 0 ]]; then
  echo "▸ building web bundle"
  pnpm --filter @agentcanvas/shared build
  pnpm --filter web build
fi
if [[ ! -f apps/web/dist/index.html ]]; then
  echo "✗ apps/web/dist/index.html missing — run without --skip-web" >&2
  exit 1
fi

echo "▸ embedding web bundle into $BUNDLE"
find "$BUNDLE" -mindepth 1 ! -name .gitignore ! -name .gitkeep -exec rm -rf {} + 2>/dev/null || true
cp -R apps/web/dist/. "$BUNDLE/"

mkdir -p dist
LDFLAGS="-s -w -X main.version=${VERSION}"

build_one() {
  local os="$1" arch="$2" out="$3"
  echo "▸ go build ${os}/${arch} → ${out}"
  (cd apps/api && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "../../${out}" ./cmd/server)
}

if [[ "$ALL" == 0 ]]; then
  host_os="$(go env GOOS)"; host_arch="$(go env GOARCH)"
  ext=""; [[ "$host_os" == windows ]] && ext=".exe"
  build_one "$host_os" "$host_arch" "dist/tandem${ext}"
  echo "✓ dist/tandem${ext} (v${VERSION})"
  exit 0
fi

rm -f dist/tandem_* dist/SHA256SUMS
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}"; arch="${target#*/}"
  name="tandem_${os}_${arch}"
  if [[ "$os" == windows ]]; then
    build_one "$os" "$arch" "dist/${name}.exe"
    (cd dist && rm -f "${name}.zip" && zip -q "${name}.zip" "${name}.exe")
  else
    build_one "$os" "$arch" "dist/${name}"
    (cd dist && tar -czf "${name}.tar.gz" "${name}")
  fi
done
(cd dist && shasum -a 256 tandem_* > SHA256SUMS)
echo "✓ dist/ (v${VERSION}):"
ls -1 dist
