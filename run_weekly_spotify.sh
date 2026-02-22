#!/bin/zsh
source /Users/benstrawbridge/.zprofile 2>/dev/null || true
source /Users/benstrawbridge/.zshrc 2>/dev/null || true
cd /Users/benstrawbridge/dev/wanderer/solo/obsidian-spotify-garden
./spotify-garden catch-up --weeks 8
./spotify-garden weekly
./spotify-garden persona
