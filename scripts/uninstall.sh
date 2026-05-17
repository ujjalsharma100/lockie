#!/usr/bin/env sh
# Remove the lockie binary installed by scripts/install.sh.
# Does not run lockie uninstall for agent hooks — do that first if needed.
set -eu

removed=0
for dir in "${LOCKIE_INSTALL_DIR:-}" "${XDG_BIN_HOME:-}" "$HOME/.local/bin" "/usr/local/bin"; do
  [ -n "$dir" ] || continue
  target="$dir/lockie"
  if [ -f "$target" ]; then
    if [ -w "$dir" ]; then
      rm -f "$target"
    else
      need_sudo=1
      command -v sudo >/dev/null 2>&1 || {
        echo "uninstall.sh: cannot remove $target (no write permission)" >&2
        exit 1
      }
      sudo rm -f "$target"
    fi
    echo "Removed $target"
    removed=1
  fi
done

if [ "$removed" -eq 0 ]; then
  echo "uninstall.sh: no lockie binary found in standard install locations" >&2
  exit 1
fi

echo "Done. Run 'lockie uninstall cursor' and 'lockie uninstall claude-code' if hooks remain."
