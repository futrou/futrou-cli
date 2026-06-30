#!/usr/bin/env bash
set -euo pipefail

# On Windows under Git Bash / MINGW, delegate to PowerShell
if [[ ${OS:-} = Windows_NT ]]; then
  if [[ ${MSYSTEM:-} != MINGW64* ]]; then
    powershell -c "irm https://futrou.com/install.ps1 | iex"
    exit $?
  fi
fi

# ---------------------------------------------------------------------------
# Colors (only when stdout is a terminal)
# ---------------------------------------------------------------------------
Color_Off=''
Red=''
Green=''
Yellow=''
Dim=''
Bold_White=''
Bold_Green=''

if [[ -t 1 ]]; then
  Color_Off='\033[0m'
  Red='\033[0;31m'
  Green='\033[0;32m'
  Yellow='\033[0;33m'
  Dim='\033[0;2m'
  Bold_Green='\033[1;32m'
  Bold_White='\033[1m'
fi

error()     { echo -e "${Red}error${Color_Off}:" "$@" >&2; exit 1; }
info()      { echo -e "${Dim}$@${Color_Off}"; }
info_bold() { echo -e "${Bold_White}$@${Color_Off}"; }
success()   { echo -e "${Bold_Green}$@${Color_Off}"; }
warn()      { echo -e "${Yellow}$@${Color_Off}"; }

# ---------------------------------------------------------------------------
# Detect platform
# ---------------------------------------------------------------------------
platform=$(uname -ms)

case $platform in
'Darwin x86_64')  target=darwin-amd64  ;;
'Darwin arm64')   target=darwin-arm64  ;;
'Linux x86_64')   target=linux-amd64   ;;
'Linux aarch64' | 'Linux arm64') target=linux-arm64 ;;
'MINGW64'*'ARM64'* | 'MINGW64'*'aarch64'*) target=windows-arm64 ;;
'MINGW64'*)       target=windows-amd64 ;;
*)
  error "Unsupported platform: $platform
Futrou CLI supports: linux/x86_64, linux/aarch64, darwin/x86_64, darwin/arm64, windows/x86_64, windows/arm64"
  ;;
esac

# Rosetta 2 detection on macOS
if [[ $target == darwin-amd64 ]]; then
  if [[ $(sysctl -n sysctl.proc_translated 2>/dev/null) == 1 ]]; then
    target=darwin-arm64
    info "Rosetta 2 detected — downloading futrou for $target instead"
  fi
fi

exe_ext=''
[[ $target == windows-* ]] && exe_ext='.exe'

# ---------------------------------------------------------------------------
# Resolve version
# ---------------------------------------------------------------------------
GITHUB=${GITHUB:-"https://github.com"}
REPO="$GITHUB/futrou/futrou-cli"

if [[ $# -eq 0 ]]; then
  version="latest"
else
  version="$1"
fi

# Normalise: accept "1.2.3" or "v1.2.3" → "v1.2.3"
if [[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  version="v$version"
fi

if [[ $version == "latest" ]]; then
  download_url="$REPO/releases/latest/download/futrou-$target$exe_ext"
else
  download_url="$REPO/releases/download/$version/futrou-$target$exe_ext"
fi

# ---------------------------------------------------------------------------
# Install location  (~/.futrou/bin/futrou)
# ---------------------------------------------------------------------------
install_env=FUTROU_INSTALL
install_dir=${FUTROU_INSTALL:-$HOME/.futrou}
bin_dir="$install_dir/bin"
exe="$bin_dir/futrou$exe_ext"

mkdir -p "$bin_dir" || error "Failed to create install directory \"$bin_dir\""

tildify() {
  [[ $1 == $HOME/* ]] && echo "~/${1#$HOME/}" || echo "$1"
}

# ---------------------------------------------------------------------------
# Detect existing installation
# ---------------------------------------------------------------------------
current_version=""
if [[ -x "$exe" ]]; then
  current_version=$("$exe" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)
fi

# ---------------------------------------------------------------------------
# Download to a temp file then atomically replace (avoids "Text file busy"
# when upgrading a running binary)
# ---------------------------------------------------------------------------
tmp_exe="$bin_dir/.futrou-tmp$exe_ext"

do_download() {
  local url="$1" dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --output "$dest" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$dest" "$url"
  else
    error "curl or wget is required to install Futrou CLI"
  fi
}

# Spinner shown while downloading
spinner_pid=""
if [[ -t 1 ]]; then
  (
    frames='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    i=0
    while true; do
      printf "\r${Dim}%s Checking versions...${Color_Off}" "${frames:$((i % ${#frames})):1}"
      sleep 0.08
      (( i++ )) || true
    done
  ) &
  spinner_pid=$!
else
  printf "${Dim}Checking versions...${Color_Off}\n"
fi

if ! do_download "$download_url" "$tmp_exe"; then
  [[ -n "$spinner_pid" ]] && kill "$spinner_pid" 2>/dev/null && printf "\r\033[K"
  rm -f "$tmp_exe"
  if [[ $version == "latest" ]]; then
    error "Failed to download latest release. Try again later.\n  $download_url"
  else
    error "Version $version not found or binary not available for $target.\n  $download_url"
  fi
fi

chmod +x "$tmp_exe"

# Stop spinner
if [[ -n "$spinner_pid" ]]; then
  kill "$spinner_pid" 2>/dev/null
  wait "$spinner_pid" 2>/dev/null || true
  printf "\r\033[K"
fi

# Read new version from downloaded binary
new_version=$("$tmp_exe" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)

# Already up to date?
if [[ -n "$current_version" && "$new_version" == "$current_version" ]]; then
  rm -f "$tmp_exe"
  success "Futrou CLI is already the latest version v$current_version."
  exit 0
fi

# Decide action label now that we know both versions
action="Installing"
if [[ -n "$current_version" && -n "$new_version" ]]; then
  IFS='.' read -r cur_maj cur_min cur_pat <<< "$current_version"
  IFS='.' read -r new_maj new_min new_pat <<< "$new_version"
  if (( new_maj > cur_maj )) || \
     (( new_maj == cur_maj && new_min > cur_min )) || \
     (( new_maj == cur_maj && new_min == cur_min && new_pat > cur_pat )); then
    action="Upgrading"
  else
    action="Downgrading"
  fi
elif [[ -n "$current_version" ]]; then
  action="Upgrading"
fi

if [[ -n "$current_version" ]]; then
  info "$action Futrou CLI v$current_version → v$new_version"
else
  info "Installing Futrou CLI v$new_version"
fi

mv -f "$tmp_exe" "$exe"

# ---------------------------------------------------------------------------
# Verify
# ---------------------------------------------------------------------------
installed_version=$("$exe" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)
[[ -z $installed_version ]] && error "Downloaded binary failed to run"

case $action in
  Upgrading)   action_past="upgraded"   ;;
  Downgrading) action_past="downgraded" ;;
  *)           action_past="installed"  ;;
esac
if [[ -n "$current_version" && "$action" != "Installing" ]]; then
  success "Futrou CLI v$current_version $action_past v$installed_version"
else
  success "Futrou CLI v$installed_version $action_past"
fi

# ---------------------------------------------------------------------------
# PATH setup (skip if already in PATH)
# ---------------------------------------------------------------------------
if command -v futrou >/dev/null 2>&1; then
  echo
  info "Run 'futrou --help' to get started"
  exit 0
fi

tilde_bin_dir=$(tildify "$bin_dir")
quoted_install_dir="\"${install_dir//\"/\\\"}\""
[[ $quoted_install_dir == \"$HOME/* ]] && quoted_install_dir="${quoted_install_dir/$HOME\//\$HOME/}"

echo

case $(basename "${SHELL:-bash}") in
fish)
  fish_config=$HOME/.config/fish/config.fish
  tilde_fish_config=$(tildify "$fish_config")
  commands=(
    "set --export $install_env $quoted_install_dir"
    "set --export PATH \$$install_env/bin \$PATH"
  )
  if [[ -w $fish_config ]]; then
    { echo; echo '# futrou'; for cmd in "${commands[@]}"; do echo "$cmd"; done; } >> "$fish_config"
    info "Added \"$tilde_bin_dir\" to \$PATH in \"$tilde_fish_config\""
    info_bold "  source $tilde_fish_config"
  else
    info "Manually add to $tilde_fish_config:"
    for cmd in "${commands[@]}"; do info_bold "  $cmd"; done
  fi
  ;;
zsh)
  zsh_config=$HOME/.zshrc
  tilde_zsh_config=$(tildify "$zsh_config")
  commands=(
    "export $install_env=$quoted_install_dir"
    "export PATH=\"\$$install_env/bin:\$PATH\""
  )
  if [[ -w $zsh_config ]]; then
    { echo; echo '# futrou'; for cmd in "${commands[@]}"; do echo "$cmd"; done; } >> "$zsh_config"
    info "Added \"$tilde_bin_dir\" to \$PATH in \"$tilde_zsh_config\""
    info_bold "  exec \$SHELL"
  else
    info "Manually add to $tilde_zsh_config:"
    for cmd in "${commands[@]}"; do info_bold "  $cmd"; done
  fi
  ;;
bash)
  commands=(
    "export $install_env=$quoted_install_dir"
    "export PATH=\"\$$install_env/bin:\$PATH\""
  )
  set_manually=true
  for bash_config in "$HOME/.bash_profile" "$HOME/.bashrc"; do
    if [[ -w $bash_config ]]; then
      { echo; echo '# futrou'; for cmd in "${commands[@]}"; do echo "$cmd"; done; } >> "$bash_config"
      info "Added \"$tilde_bin_dir\" to \$PATH in \"$(tildify "$bash_config")\""
      info_bold "  source $(tildify "$bash_config")"
      set_manually=false
      break
    fi
  done
  if [[ $set_manually == true ]]; then
    info "Manually add to ~/.bashrc:"
    for cmd in "${commands[@]}"; do info_bold "  $cmd"; done
  fi
  ;;
*)
  info "Manually add \"$tilde_bin_dir\" to your \$PATH:"
  info_bold "  export $install_env=$quoted_install_dir"
  info_bold "  export PATH=\"\$$install_env/bin:\$PATH\""
  ;;
esac

echo
info "Reload your shell to use futrou:"
info_bold "  exec \$SHELL"
echo
info "Or open a new terminal and run:"
info_bold "  futrou --help"

# Make futrou available in the current shell session without restarting
export PATH="$bin_dir:$PATH"
