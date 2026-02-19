#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist/windows"
RUNTIME_DIR="${DIST_DIR}/runtime/windows"
SING_BOX_ZIP_URL="${SING_BOX_ZIP_URL:-}"
SING_BOX_EXE_PATH="${SING_BOX_EXE_PATH:-}"
ZIG_VERSION="${ZIG_VERSION:-0.14.0}"
AUTO_DOWNLOAD_ZIG="${AUTO_DOWNLOAD_ZIG:-1}"

mkdir -p "${DIST_DIR}" "${RUNTIME_DIR}"
cd "${ROOT_DIR}"

resolve_sing_box_zip_url() {
  if [[ -n "${SING_BOX_ZIP_URL}" ]]; then
    printf '%s\n' "${SING_BOX_ZIP_URL}"
    return 0
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "[error] curl is required to resolve latest sing-box release URL." >&2
    return 1
  fi
  local release_json
  release_json="$(curl -fsSL "https://api.github.com/repos/SagerNet/sing-box/releases/latest")"
  local url
  url="$(printf '%s' "${release_json}" | grep -oE 'https://[^"]*sing-box-[^"]*-windows-amd64\.zip' | head -n1 || true)"
  if [[ -z "${url}" ]]; then
    echo "[error] unable to resolve sing-box windows-amd64 zip from latest release." >&2
    return 1
  fi
  printf '%s\n' "${url}"
}

echo "[1/6] Downloading Go modules..."
go mod download

echo "[2/6] Checking C compiler for Fyne (CGO required)..."
if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
  export CC=x86_64-w64-mingw32-gcc
elif command -v gcc >/dev/null 2>&1; then
  export CC=gcc
elif [[ -x "${ROOT_DIR}/.tools/zig-${ZIG_VERSION}/zig" ]]; then
  export CC="${ROOT_DIR}/.tools/zig-${ZIG_VERSION}/zig cc"
elif command -v zig >/dev/null 2>&1; then
  export CC="zig cc"
else
  if [[ "${AUTO_DOWNLOAD_ZIG}" != "1" ]]; then
    echo "[error] C compiler not found. Install gcc or set AUTO_DOWNLOAD_ZIG=1."
    exit 1
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "[error] curl is required to auto-download zig compiler."
    exit 1
  fi
  if ! command -v tar >/dev/null 2>&1; then
    echo "[error] tar is required to auto-download zig compiler."
    exit 1
  fi
  echo "      downloading portable zig ${ZIG_VERSION}..."
  mkdir -p "${ROOT_DIR}/.tools"
  ZIG_ARCHIVE="${ROOT_DIR}/.tools/zig-linux-x86_64-${ZIG_VERSION}.tar.xz"
  ZIG_DIR="${ROOT_DIR}/.tools/zig-${ZIG_VERSION}"
  curl -fL "https://ziglang.org/download/${ZIG_VERSION}/zig-linux-x86_64-${ZIG_VERSION}.tar.xz" -o "${ZIG_ARCHIVE}"
  rm -rf "${ZIG_DIR}" "${ROOT_DIR}/.tools/zig-linux-x86_64-${ZIG_VERSION}"
  tar -xf "${ZIG_ARCHIVE}" -C "${ROOT_DIR}/.tools"
  mv "${ROOT_DIR}/.tools/zig-linux-x86_64-${ZIG_VERSION}" "${ZIG_DIR}"
  export CC="${ZIG_DIR}/zig cc"
fi

echo "[3/6] Building vpnclient.exe..."
export CGO_ENABLED=1
GOOS=windows GOARCH=amd64 go build -o "${DIST_DIR}/vpnclient.exe" ./cmd/gui

echo "[4/6] Preparing sing-box runtime binary..."
if [[ -n "${SING_BOX_EXE_PATH}" ]]; then
  cp "${SING_BOX_EXE_PATH}" "${RUNTIME_DIR}/sing-box.exe"
else
  if ! command -v curl >/dev/null 2>&1; then
    echo "[error] curl is required to download sing-box runtime."
    exit 1
  fi
  if ! command -v unzip >/dev/null 2>&1; then
    echo "[error] unzip is required to extract sing-box runtime."
    exit 1
  fi
  ZIP_URL="$(resolve_sing_box_zip_url)"
  TMP_DIR="${ROOT_DIR}/.tmp/build-windows-runtime"
  ZIP_PATH="${TMP_DIR}/sing-box-windows-amd64.zip"
  EXTRACT_DIR="${TMP_DIR}/extract"
  rm -rf "${TMP_DIR}"
  mkdir -p "${EXTRACT_DIR}"
  curl -fL "${ZIP_URL}" -o "${ZIP_PATH}"
  unzip -q -o "${ZIP_PATH}" -d "${EXTRACT_DIR}"
  BIN_PATH="$(find "${EXTRACT_DIR}" -type f -name 'sing-box.exe' | head -n1 || true)"
  if [[ -z "${BIN_PATH}" ]]; then
    echo "[error] sing-box.exe not found in downloaded archive."
    exit 1
  fi
  cp "${BIN_PATH}" "${RUNTIME_DIR}/sing-box.exe"
fi

echo "[5/6] Copying docs..."
cp "${ROOT_DIR}"/*.md "${DIST_DIR}/"
if [[ -f "${ROOT_DIR}/client.settings.json" ]]; then
  cp "${ROOT_DIR}/client.settings.json" "${DIST_DIR}/client.settings.json"
fi

echo "[6/6] Creating zip package..."
if command -v zip >/dev/null 2>&1; then
  rm -f "${ROOT_DIR}/dist/vpnclient-windows-amd64.zip"
  (
    cd "${DIST_DIR}"
    zip -r "../vpnclient-windows-amd64.zip" .
  )
else
  echo "[warn] zip not found, skipping archive creation."
fi

echo "Done. Artifacts:"
echo " - ${DIST_DIR}/vpnclient.exe"
echo " - ${DIST_DIR}/runtime/windows/sing-box.exe"
echo " - ${ROOT_DIR}/dist/vpnclient-windows-amd64.zip (if zip installed)"
