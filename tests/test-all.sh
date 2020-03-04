#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

export CLI_LOGFILE="/tmp/test-build.log"
export CLI_LOGFILE_ALTERNATE="/tmp/test-build-alternate.log"
export CLI_CONFIGFILE="/etc/cronitor/cronitor.json"
export CLI_CONFIGFILE_ALTERNATE="/tmp/test-build-config.json"
export CLI_ACTUAL_API_KEY="cb54ac4fd16142469f2d84fc1bbebd84"
export CLI_CRONTAB_TEMP="/tmp/crontab"

# First, build fresh
cd ../
go build && echo "Build created.. OK"
cd - > /dev/null

#./test-configure.sh
./test-discover.sh
#./test-exec.sh
#./test-list.sh
#./test-ping.sh
#./test-select.sh
#./test-shell.sh
#./test-status.sh
