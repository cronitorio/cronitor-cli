#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

source ./setup.sh

#################
# PING TESTS
#################

echo ""

rm -f $CLI_LOGFILE
TEST="Ping without args"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $CLI_LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Ping with custom hostname"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --hostname customHostnameForTest --log $CLI_LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $CLI_LOGFILE && grep -q "host=customHostnameForTest" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Ping with message"
MSG="messagewithoutspaces"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --msg "$MSG" --log $CLI_LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $CLI_LOGFILE && grep -q "$MSG" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Ping with ping api key"
KEY="XXXXXXXXXX"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --ping-api-key $KEY --log $CLI_LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $CLI_LOGFILE && grep -q "auth_key=${KEY}" $CLI_LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $CLI_LOGFILE
TEST="Ping integration test"
MSG=`date`
../cronitor $CRONITOR_ARGS ping 44oI2n --run --msg "$MSG" --log $CLI_LOGFILE && sleep 1
if ../cronitor $CRONITOR_ARGS activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
