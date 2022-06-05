#!/usr/bin/env bash

echo "Running test-status..."

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

source ./setup.sh

#################
# STATUS TESTS
#################

echo ""

rm -f $CLI_LOGFILE
TEST="Status integration test without filter"
if ../cronitor $CRONITOR_ARGS status --log $CLI_LOGFILE | grep -q "Ok"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Status integration test with filter"
if ../cronitor $CRONITOR_ARGS status 44oI2n --log $CLI_LOGFILE | grep -q "Ok"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Status integration test with bad monitor code"
if ../cronitor $CRONITOR_ARGS status asdfgh --log $CLI_LOGFILE 2>&1 | grep -q "404"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
