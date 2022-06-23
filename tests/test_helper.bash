#!/usr/bin/bash

skip_if_windows() {
    if [ "$WINDOWS" = "true" ] ; then
        skip "Skipping this test on Windows"
    fi
}

skip_until_key_returned() {
    skip 'Skipping until newer API is updated to return `key`'
}