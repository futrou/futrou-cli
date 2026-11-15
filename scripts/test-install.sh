#!/bin/bash
# Tests that install.sh adds futrou to a shell config only once, however often it runs.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# the functions under test, taken from the installer itself
extract() { sed -n "/^$1() {/,/^}/p" "$root/install.sh"; }
for fn in tildify configures_futrou append_futrou_config; do
  body=$(extract "$fn")
  [[ -n $body ]] || { echo "FAIL: $fn not found in install.sh"; exit 1; }
  eval "$body"
done

failures=0
check() { # description, command...
  local description=$1
  shift
  if "$@"; then echo "ok   $description"; else echo "FAIL $description"; failures=$((failures + 1)); fi
}
not() { ! "$@"; }
count() { grep -c "$2" "$1" || true; }

export HOME="$work/home"
mkdir -p "$HOME"
install_env=FUTROU_INSTALL
install_dir=$HOME/.futrou
bin_dir=$install_dir/bin
quoted_install_dir='"$HOME/.futrou"'
commands=("export $install_env=$quoted_install_dir" "export PATH=\"\$$install_env/bin:\$PATH\"")

rc=$HOME/.bashrc
touch "$rc"
check "a missing config is not configured" not configures_futrou $HOME/missing
check "an empty config is not configured" not configures_futrou $rc

append_futrou_config "$rc" "${commands[@]}"
check "the first run adds the block" test "$(count "$rc" '^# futrou$')" = 1
check "the block sets FUTROU_INSTALL" test "$(count "$rc" '^export FUTROU_INSTALL=')" = 1
check "the block adds the bin folder to PATH" test "$(count "$rc" 'FUTROU_INSTALL/bin')" = 1

before=$(cat "$rc")
if append_futrou_config "$rc" "${commands[@]}"; then added=yes; else added=no; fi
check "a second run reports that nothing was added" test "$added" = no
check "a second run leaves the file as it was" test "$(cat "$rc")" = "$before"

for _ in 1 2 3 4 5; do append_futrou_config "$rc" "${commands[@]}" || true; done
check "five more runs still leave one block" test "$(count "$rc" '^# futrou$')" = 1
check "and one FUTROU_INSTALL line" test "$(count "$rc" 'export FUTROU_INSTALL=')" = 1

# a config the user wrote by hand counts too
manual=$HOME/.zshrc
printf 'export PATH="$HOME/.futrou/bin:$PATH"\n' > "$manual"
check "a hand written PATH line with \$HOME is recognised" configures_futrou "$manual"
printf 'export PATH="%s:$PATH"\n' "$bin_dir" > "$manual"
check "a hand written PATH line with the full path is recognised" configures_futrou "$manual"
printf 'export PATH="~/.futrou/bin:$PATH"\n' > "$manual"
check "a hand written PATH line with a tilde is recognised" configures_futrou "$manual"
printf 'export FUTROU_INSTALL=/opt/futrou\n' > "$manual"
check "a FUTROU_INSTALL set by hand is recognised" configures_futrou "$manual"
if append_futrou_config "$manual" "${commands[@]}"; then added=yes; else added=no; fi
check "nothing is added to a config that has it by hand" test "$added" = no

# unrelated lines are not mistaken for it
other=$HOME/.profile
printf 'export FUTROU_INSTALLER=1\nexport MY_FUTROU_INSTALL=2\nexport PATH="$HOME/bin:$PATH"\n# futrou is great\n' > "$other"
check "similar names are not recognised" not configures_futrou $other
append_futrou_config "$other" "${commands[@]}"
check "so the block is added to such a file" test "$(count "$other" '^export FUTROU_INSTALL=')" = 1

# a custom install folder
install_dir=$work/custom
bin_dir=$install_dir/bin
custom=$HOME/.custom_rc
printf 'export PATH="%s:$PATH"\n' "$bin_dir" > "$custom"
check "a custom install folder in PATH is recognised" configures_futrou "$custom"
other_custom=$HOME/.other_rc
printf 'export PATH="%s:$PATH"\n' "$work/elsewhere/bin" > "$other_custom"
check "another folder is not" not configures_futrou $other_custom

# the installer itself must be valid and use the guard in every shell branch
check "install.sh is valid bash" bash -n "$root/install.sh"
check "no branch appends to a config without the guard" test "$(grep -c '>> "\$' "$root/install.sh")" = 1
check "every shell branch checks the config first" test "$(grep -c 'if configures_futrou' "$root/install.sh")" -ge 3

if [[ $failures -gt 0 ]]; then
  echo "$failures install test(s) failed"
  exit 1
fi
echo "install.sh tests passed"
