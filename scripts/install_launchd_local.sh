#!/bin/zsh
set -euo pipefail

# Installs/updates spotify-garden in stable user-local paths for launchd.
# Safe to re-run after pulling new changes; this is the upgrade path.

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd -P)"
BIN_DIR="${HOME}/.local/bin"
APP_DIR="${HOME}/Library/Application Support/spotify-garden"
STATE_DIR="${APP_DIR}/state"
TEMPLATES_DIR="${APP_DIR}/templates"
LOG_DIR="${APP_DIR}/logs"
LAUNCH_DIR="${HOME}/Library/LaunchAgents"

COLLECT_LABEL="${SPOTIFY_COLLECT_LABEL:-com.${USER}.spotify-collect}"
WEEKLY_LABEL="${SPOTIFY_WEEKLY_LABEL:-com.${USER}.spotify-weekly}"

COLLECT_WRAPPER="${BIN_DIR}/spotify-garden-collect.sh"
WEEKLY_WRAPPER="${BIN_DIR}/spotify-garden-weekly.sh"
BIN_PATH="${BIN_DIR}/spotify-garden"

COLLECT_PLIST="${LAUNCH_DIR}/${COLLECT_LABEL}.plist"
WEEKLY_PLIST="${LAUNCH_DIR}/${WEEKLY_LABEL}.plist"

echo "==> Preparing directories"
mkdir -p "${BIN_DIR}" "${STATE_DIR}" "${TEMPLATES_DIR}" "${LOG_DIR}" "${LAUNCH_DIR}"
mkdir -p "${STATE_DIR}/data"

echo "==> Building binary"
cd "${REPO_DIR}"
go build -o "${BIN_PATH}" .

echo "==> Syncing templates"
rm -rf "${TEMPLATES_DIR}"
cp -R "${REPO_DIR}/templates" "${TEMPLATES_DIR}"

if [[ ! -f "${STATE_DIR}/.env" && -f "${REPO_DIR}/.env" ]]; then
  echo "==> Copying initial .env into state dir"
  cp "${REPO_DIR}/.env" "${STATE_DIR}/.env"
fi

if [[ ! -f "${STATE_DIR}/tokens.json" && -f "${REPO_DIR}/tokens.json" ]]; then
  echo "==> Copying initial tokens.json into state dir"
  cp "${REPO_DIR}/tokens.json" "${STATE_DIR}/tokens.json"
  chmod 600 "${STATE_DIR}/tokens.json"
fi

echo "==> Writing wrappers"
cat > "${COLLECT_WRAPPER}" <<EOF
#!/bin/zsh
set -euo pipefail
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
export SPOTIFY_TEMPLATES_DIR="${TEMPLATES_DIR}"
export SPOTIFY_AUTO_DAILY_ON_COLLECT=1
cd "${STATE_DIR}"
exec "${BIN_PATH}" collect
EOF
chmod +x "${COLLECT_WRAPPER}"

cat > "${WEEKLY_WRAPPER}" <<EOF
#!/bin/zsh
set -euo pipefail
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
export SPOTIFY_TEMPLATES_DIR="${TEMPLATES_DIR}"
cd "${STATE_DIR}"
"${BIN_PATH}" catch-up --weeks 8
"${BIN_PATH}" weekly
exec "${BIN_PATH}" persona
EOF
chmod +x "${WEEKLY_WRAPPER}"

echo "==> Writing LaunchAgents"
cat > "${COLLECT_PLIST}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${COLLECT_LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/zsh</string>
    <string>${COLLECT_WRAPPER}</string>
  </array>
  <key>WorkingDirectory</key>
  <string>${STATE_DIR}</string>
  <key>StartCalendarInterval</key>
  <array>
    <dict><key>Hour</key><integer>7</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Hour</key><integer>11</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Hour</key><integer>15</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Hour</key><integer>19</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Hour</key><integer>23</integer><key>Minute</key><integer>0</integer></dict>
  </array>
  <key>StandardOutPath</key>
  <string>${LOG_DIR}/collect.log</string>
  <key>StandardErrorPath</key>
  <string>${LOG_DIR}/collect.log</string>
</dict>
</plist>
EOF

cat > "${WEEKLY_PLIST}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${WEEKLY_LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/zsh</string>
    <string>${WEEKLY_WRAPPER}</string>
  </array>
  <key>WorkingDirectory</key>
  <string>${STATE_DIR}</string>
  <key>StartCalendarInterval</key>
  <array>
    <dict>
      <key>Weekday</key><integer>0</integer>
      <key>Hour</key><integer>23</integer>
      <key>Minute</key><integer>0</integer>
    </dict>
  </array>
  <key>StandardOutPath</key>
  <string>${LOG_DIR}/weekly.log</string>
  <key>StandardErrorPath</key>
  <string>${LOG_DIR}/weekly.log</string>
</dict>
</plist>
EOF

echo "==> Validating plists"
plutil -lint "${COLLECT_PLIST}" "${WEEKLY_PLIST}" >/dev/null

UID_NUM="$(id -u)"
COLLECT_TARGET="gui/${UID_NUM}/${COLLECT_LABEL}"
WEEKLY_TARGET="gui/${UID_NUM}/${WEEKLY_LABEL}"

echo "==> Reloading LaunchAgents"
launchctl bootout "gui/${UID_NUM}" "${COLLECT_PLIST}" 2>/dev/null || true
launchctl bootout "gui/${UID_NUM}" "${WEEKLY_PLIST}" 2>/dev/null || true
launchctl bootstrap "gui/${UID_NUM}" "${COLLECT_PLIST}"
launchctl bootstrap "gui/${UID_NUM}" "${WEEKLY_PLIST}"
launchctl kickstart -k "${COLLECT_TARGET}"

echo
echo "Install complete."
echo "Binary:     ${BIN_PATH}"
echo "State dir:  ${STATE_DIR}"
echo "Logs:       ${LOG_DIR}"
echo "Collect:    ${COLLECT_TARGET}"
echo "Weekly:     ${WEEKLY_TARGET}"
echo
echo "Upgrade path: after git pull, re-run:"
echo "  ${REPO_DIR}/scripts/install_launchd_local.sh"
