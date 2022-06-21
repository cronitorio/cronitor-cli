#!/usr/bin/env bash

setup_suite() {
    export CLI_LOGFILE="/tmp/test-build.log"
    export CLI_LOGFILE_ALTERNATE="/tmp/test-build-alternate.log"
    export CLI_CONFIGFILE="/etc/cronitor/cronitor.json"
    #CLI_CONFIGFILE="/tmp/cronitor.json"
    export CLI_CONFIGFILE_ALTERNATE="/tmp/test-build-config.json"
    #CLI_ACTUAL_API_KEY="cb54ac4fd16142469f2d84fc1bbebd84"
    export CLI_ACTUAL_API_KEY="$CRONITOR_API_KEY"
    export CLI_CRONTAB_TEMP="/tmp/crontab"
    export CLI_USERNAME=`whoami`

    if [ "$1" = "--use-dev" ]
        then
            export CRONITOR_ARGS="--use-dev"
            export HOSTNAME="http://localhost:8000"
        else
            export CRONITOR_ARGS=""
            export HOSTNAME="https://cronitor.link"
    fi

    ../cronitor configure -k "$CLI_ACTUAL_API_KEY" >/dev/null 2>/dev/null
}