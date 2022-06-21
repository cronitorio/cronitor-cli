#!/usr/bin/bash

skip_if_windows() {
    if [ "$WINDOWS" = "true" ] ; then
        skip "Skipping this test on Windows"
    fi
}