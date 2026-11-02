#!/bin/bash
# Signs release files with the Futrou release key: one detached, ASCII-armored
# signature per file, written next to it as <file>.asc.
#
#   scripts/sign.sh [file...]      (default: every file in $DIST_DIR except .asc and SHA256SUMS)
#
# Without arguments it also writes $DIST_DIR/SHA256SUMS (sha256 of every signed
# file, in `sha256sum -c` format) and signs it as SHA256SUMS.asc, so a download
# can be checked with the checksum file as well as with its own signature.
#
# Environment (CI secrets):
#   GPG_PRIVATE_KEY_BASE64   the armored secret key, base64 encoded without line
#                            breaks: gpg --armor --export-secret-keys <id> | base64 -w0
#   GPG_PASSPHRASE           its passphrase (leave unset for a key without one)
set -euo pipefail

DIST_DIR="${DIST_DIR:-dist}"

: "${GPG_PRIVATE_KEY_BASE64:?GPG_PRIVATE_KEY_BASE64 is not set}"
command -v gpg >/dev/null || { echo "gpg is not installed" >&2; exit 1; }

if [ "$#" -gt 0 ]; then
    FILES=("$@")
    WRITE_SUMS=0
else
    mapfile -t FILES < <(find "$DIST_DIR" -maxdepth 1 -type f ! -name '*.asc' ! -name SHA256SUMS | sort)
    WRITE_SUMS=1
fi
[ "${#FILES[@]}" -gt 0 ] || { echo "nothing to sign" >&2; exit 1; }

# A keyring of its own, removed on exit; the secret key never touches ~/.gnupg.
GNUPGHOME="$(mktemp -d)"
export GNUPGHOME
chmod 700 "$GNUPGHOME"
trap 'gpgconf --kill gpg-agent >/dev/null 2>&1 || true; rm -rf "$GNUPGHOME"' EXIT

echo "$GPG_PRIVATE_KEY_BASE64" | base64 -d | gpg --batch --quiet --import
FINGERPRINT="$(gpg --batch --list-secret-keys --with-colons | awk -F: '$1 == "fpr" { print $10; exit }')"
[ -n "$FINGERPRINT" ] || { echo "no secret key imported" >&2; exit 1; }

sign_file() {
    local file="$1"
    echo "Signing $file..."
    rm -f "$file.asc"
    if [ -n "${GPG_PASSPHRASE:-}" ]; then
        gpg --batch --yes --quiet --pinentry-mode loopback --passphrase-fd 3 --local-user "$FINGERPRINT" --armor --detach-sign --output "$file.asc" "$file" 3<<<"$GPG_PASSPHRASE"
    else
        gpg --batch --yes --quiet --local-user "$FINGERPRINT" --armor --detach-sign --output "$file.asc" "$file"
    fi
    gpg --batch --quiet --verify "$file.asc" "$file"
}

for FILE in "${FILES[@]}"; do
    sign_file "$FILE"
done

if [ "$WRITE_SUMS" = "1" ]; then
    SUMS="$DIST_DIR/SHA256SUMS"
    (cd "$DIST_DIR" && sha256sum -- "${FILES[@]##*/}") > "$SUMS"
    sign_file "$SUMS"
    (cd "$DIST_DIR" && sha256sum --check --quiet SHA256SUMS)
fi

echo "✅ Signed ${#FILES[@]} file(s)$([ "$WRITE_SUMS" = "1" ] && echo " and SHA256SUMS") with key $FINGERPRINT"
