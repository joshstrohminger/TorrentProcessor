#!/bin/bash

set -euo pipefail

# check if it's already installed
if command -v TorrentProcessor &> /dev/null; then
    # ensure we don't try anything while the daemon is running
    if ! go run . process daemon --exit-code; then
        echo "TorrentProcessor is currently running, wait until it has stopped and try again"
        exit 1
    fi

    # disable the daemon, ignoring errors
    TorrentProcessor process daemon stop || true
fi

APP_DIR=$(dirname $(TorrentProcessor config get workpath))

rm $(which TorrentProcessor)

# leave config, logs, and work unless told to delete it all
case "$1" in
    clean|remove|all|cleanup)
        rm -r $"APP_DIR"
        ;;
esac

