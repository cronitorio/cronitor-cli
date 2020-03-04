#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

if [ "$1" = "--use-dev" ]
    then
        CRONITOR_ARGS="--use-dev"
        HOSTNAME="http://localhost:8000"
    else
        CRONITOR_ARGS=""
        HOSTNAME="https://cronitor.link"
fi

#################
# LIST TESTS
#################

echo ""

rm -f $CLI_LOGFILE
TEST="List reads crontab and writes table"
if ../cronitor $CRONITOR_ARGS list ../fixtures/crontab.txt | grep -q "babylist_web"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="List reads crontab and formats table correctly"
if ../cronitor $CRONITOR_ARGS list ../fixtures/crontab.txt | grep -q "\-----"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
