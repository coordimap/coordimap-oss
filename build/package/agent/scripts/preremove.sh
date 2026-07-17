#!/bin/sh
# $1 is "remove" or "upgrade" (deb)
# $1 is 0 or 1 (rpm)

if [ "$1" = "remove" ] || [ "$1" = "0" ]; then
    systemctl stop coordimap-agent || true
    systemctl disable coordimap-agent || true
fi
