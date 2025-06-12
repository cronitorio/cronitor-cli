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
    #CLI_CONFIGFILE="/tmp/cronitor.json"
    export CLI_CONFIGFILE_ALTERNATE="$BATS_TMPDIR/test-build-config.json"
    #CLI_ACTUAL_API_KEY="cb54ac4fd16142469f2d84fc1bbebd84"
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

    echo "Attempting to configure cronitor with API key..."
    echo "API key length: ${#CLI_ACTUAL_API_KEY}"
    echo "Binary exists: $(test -f ../cronitor && echo 'yes' || echo 'no')"
    echo "Binary permissions: $(ls -la ../cronitor 2>/dev/null || echo 'not found')"
    ../cronitor configure -k "$CLI_ACTUAL_API_KEY"
}
