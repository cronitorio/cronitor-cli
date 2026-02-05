#!/usr/bin/bash

skip_if_windows() {
    if [ "$WINDOWS" = "true" ] ; then
        SKIP_MESSAGE="Skipping this test on Windows"
        if [ "$1" != "" ] ; then
            SKIP_MESSAGE="$SKIP_MESSAGE: $1"
        fi
        skip "$SKIP_MESSAGE"
    fi
}

skip_if_linux() {
    if [ "$WINDOWS" = "false" ] ; then
        skip "Skipping this test on Linux"
    fi
}