#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

source ./setup.sh

#################
# ACTIVITY TESTS
#################

echo ""

rm -f $CLI_LOGFILE
TEST="Activity integration test without filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --log $CLI_LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Activity integration test with only pings filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --only pings --log $CLI_LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Activity integration test with only alerts filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --only alerts --log $CLI_LOGFILE | grep -q -v "\"description\": \"ping\""
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
