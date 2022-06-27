#!/usr/bin/bash

skip_if_windows() {
    if [ "$WINDOWS" = "true" ] ; then
        skip "Skipping this test on Windows"
    fi
}

skip_if_linux() {
    if [ "$WINDOWS" = "false" ] ; then
        skip "Skipping this test on Linux"
    fi
}