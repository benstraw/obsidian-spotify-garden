#!/bin/zsh
source /Users/benstrawbridge/.zprofile 2>/dev/null || true
source /Users/benstrawbridge/.zshrc 2>/dev/null || true
cd /Users/benstrawbridge/dev/wanderer/solo/obsidian-spotify-garden
exec ./spotify-garden collect
