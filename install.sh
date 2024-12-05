#!/bin/bash

set -euo pipefail

if command -v TorrentProcessor &> /dev/null; then
    # ensure we don't try anything while a daemon is running
    if ! go run . process daemon --exit-code; then
        echo "TorrentProcessor is currently running, wait until it has stopped and try again"
        exit 1
    fi

    # disable the daemon, ignoring errors
    TorrentProcessor process daemon stop || true
fi

go install -v .
TorrentProcessor setup
TorrentProcessor process daemon start
