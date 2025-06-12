#!/usr/bin/bash

setup_suite() {
    bats_require_minimum_version 1.5.0

    export CLI_LOGFILE="$BATS_TMPDIR/test-build.log"
    export CLI_LOGFILE_ALTERNATE="$BATS_TMPDIR/test-build-alternate.log"
    if [ "$WINDOWS" = "true" ] ; then
        export CLI_CONFIGFILE="C:\ProgramData\Cronitor\cronitor.json"
        export CLI_CRONTAB_TEMP="C:\Users\runneradmin\AppData\Local\Temp\crontab.txt"
    else
        export CLI_CONFIGFILE="/etc/cronitor/cronitor.json"
        export CLI_CRONTAB_TEMP="$BATS_TMPDIR/crontab.txt"
    fi
    export CLI_CONFIGFILE_ALTERNATE="$BATS_TMPDIR/test-build-config.json"
    export CLI_ACTUAL_API_KEY="$CRONITOR_API_KEY"
    export CLI_USERNAME=`whoami`

    if [ "$1" = "--use-dev" ]
        then
            export CRONITOR_ARGS="--use-dev"
            export HOSTNAME="http://localhost:8000"
        else
            export CRONITOR_ARGS=""
            export HOSTNAME="https://cronitor.link"
    fi

    ../cronitor configure --api-key "$CLI_ACTUAL_API_KEY" >/dev/null 2>/dev/null
}
