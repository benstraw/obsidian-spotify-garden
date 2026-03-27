#!/bin/zsh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$HOME/.zprofile" 2>/dev/null || true
source "$HOME/.zshrc" 2>/dev/null || true
cd "$SCRIPT_DIR"
export MUSIC_AUTO_DAILY_ON_COLLECT_SPOTIFY=1
exec ./music-garden collect
