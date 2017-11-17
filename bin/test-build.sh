#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

LOGFILE="/tmp/test-build.log"

#################
# PING TESTS
#################

# Test basic ping
rm $LOGFILE
TEST="Basic ping check"
../cronitor ping d3x0c1 --run --log /tmp/test-build.log
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed" && exit 1
fi

# Test using a custom hostname
rm $LOGFILE
TEST="Ping check with custom hostname"
../cronitor ping d3x0c1 --run --hostname customHostnameForTest --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q "hostname=customHostnameForTest" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed" && exit 1
fi

rm $LOGFILE
TEST="Ping check with message"
MSG="messagewithoutspaces"
../cronitor ping d3x0c1 --run --msg "$MSG" --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q "$MSG" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed" && exit 1
fi

# Test with a ping api key
rm $LOGFILE
TEST="Ping check api key"
KEY="XXXXXXXXXX"
../cronitor ping d3x0c1 --run --ping-api-key $KEY --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q $KEY $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed" && exit 1
fi


#################
# EXEC TESTS
#################


rm $LOGFILE
TEST="Exec runs command check"
../cronitor --log $LOGFILE exec d3x0c1 ./write-to-log-success.sh $LOGFILE "$TEST"
if grep -q "$TEST" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed" && exit 1
fi

rm $LOGFILE
TEST="Exec parses input flags correctly"
../cronitor exec d3x0c1 --log $LOGFILE ./write-to-log-success.sh $LOGFILE "$TEST"
if grep -sq "$TEST" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed" && exit 1
fi
