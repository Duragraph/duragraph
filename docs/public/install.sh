#!/bin/sh
# DuraGraph installer.
#
# Usage:
#   curl -fsSL https://duragraph.ai/install.sh | sh
#
# Environment overrides:
#   DURAGRAPH_VERSION       Pin to a specific tag, e.g. v0.7.5 (default: latest)
#   DURAGRAPH_INSTALL_DIR   Install location (default: /usr/local/bin if writable,
#                           else $HOME/.local/bin)

set -eu

REPO="Duragraph/duragraph"
BINARY="duragraph"

# ---------- helpers ---------------------------------------------------------

err() {
    printf 'install.sh: error: %s\n' "$*" >&2
    exit 1
}

info() {
    printf 'install.sh: %s\n' "$*" >&2
}

need_cmd() {
    command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

# ---------- detection -------------------------------------------------------

detect_os() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        darwin) printf 'darwin' ;;
        linux)  printf 'linux' ;;
        *)      err "unsupported OS: $(uname -s) (only macOS and Linux are supported)" ;;
    esac
}

detect_arch() {
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)  printf 'x86_64' ;;
        arm64|aarch64) printf 'arm64' ;;
        *)             err "unsupported architecture: $arch (only x86_64 and arm64 are supported)" ;;
    esac
}

# Resolve the latest release tag via GitHub's redirect (no API rate limit).
latest_version() {
    redirect=$(curl -sILo /dev/null -w '%{url_effective}' \
        "https://github.com/${REPO}/releases/latest")
    case "$redirect" in
        */tag/v*) printf '%s' "${redirect##*/tag/}" ;;
        *)        err "could not determine latest version from redirect: $redirect" ;;
    esac
}

# ---------- checksum --------------------------------------------------------

verify_checksum() {
    archive=$1
    expected=$2

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$archive" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$archive" | awk '{print $1}')
    else
        err "neither sha256sum nor shasum available — cannot verify download integrity"
    fi

    [ "$actual" = "$expected" ] || err "checksum mismatch (expected $expected, got $actual)"
}

# ---------- install dir -----------------------------------------------------

choose_install_dir() {
    if [ -n "${DURAGRAPH_INSTALL_DIR:-}" ]; then
        printf '%s' "$DURAGRAPH_INSTALL_DIR"
        return
    fi
    if [ -w /usr/local/bin ]; then
        printf '/usr/local/bin'
        return
    fi
    printf '%s/.local/bin' "$HOME"
}

# ---------- main ------------------------------------------------------------

main() {
    need_cmd curl
    need_cmd tar
    need_cmd uname
    need_cmd awk

    os=$(detect_os)
    arch=$(detect_arch)

    if [ -n "${DURAGRAPH_VERSION:-}" ]; then
        version=$DURAGRAPH_VERSION
    else
        version=$(latest_version)
    fi
    # Normalize: always ensure leading 'v' on the tag
    case "$version" in
        v*) ;;
        *)  version="v${version}" ;;
    esac
    version_no_v="${version#v}"

    archive_name="${BINARY}_${version_no_v}_${os}_${arch}.tar.gz"
    archive_url="https://github.com/${REPO}/releases/download/${version}/${archive_name}"
    checksum_url="https://github.com/${REPO}/releases/download/${version}/checksums.txt"

    install_dir=$(choose_install_dir)

    info "platform: ${os}/${arch}"
    info "version:  ${version}"
    info "target:   ${install_dir}/${BINARY}"

    tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t duragraph-install)
    trap 'rm -rf "$tmpdir"' EXIT

    archive_path="${tmpdir}/${archive_name}"

    info "downloading ${archive_url}"
    curl -fsSL --retry 3 "$archive_url" -o "$archive_path" \
        || err "download failed: $archive_url"

    info "verifying checksum"
    checksums=$(curl -fsSL --retry 3 "$checksum_url") \
        || err "could not fetch checksums.txt"
    expected=$(printf '%s\n' "$checksums" | awk -v f="$archive_name" '$2==f {print $1}')
    [ -n "$expected" ] || err "no checksum entry for ${archive_name} in checksums.txt"
    verify_checksum "$archive_path" "$expected"

    info "extracting"
    tar -xzf "$archive_path" -C "$tmpdir" || err "tar extraction failed"
    [ -f "${tmpdir}/${BINARY}" ] || err "extracted archive does not contain '${BINARY}'"

    mkdir -p "$install_dir" || err "could not create ${install_dir}"
    info "installing"
    if command -v install >/dev/null 2>&1; then
        install -m 0755 "${tmpdir}/${BINARY}" "${install_dir}/${BINARY}" \
            || err "install to ${install_dir} failed"
    else
        mv "${tmpdir}/${BINARY}" "${install_dir}/${BINARY}" \
            || err "move to ${install_dir} failed"
        chmod 0755 "${install_dir}/${BINARY}"
    fi

    # PATH guidance — non-fatal, just a heads-up
    case ":${PATH:-}:" in
        *":${install_dir}:"*)
            ;;
        *)
            info ""
            info "NOTE: ${install_dir} is not in your PATH. Add it to your shell profile:"
            info "  export PATH=\"${install_dir}:\$PATH\""
            ;;
    esac

    cat >&2 <<EOF

duragraph ${version} installed at ${install_dir}/${BINARY}

Quick start — zero config, embedded Postgres + NATS:
  duragraph dev

Self-hosted serve mode (config in ~/.config/duragraph/config.toml or env vars):
  duragraph serve

Documentation: https://duragraph.ai
EOF
}

main "$@"
