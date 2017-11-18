#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

LOGFILE="/tmp/test-build.log"

#################
# PING TESTS
#################

# Test basic ping
rm $LOGFILE
TEST="Ping without args"
../cronitor ping d3x0c1 --run --log /tmp/test-build.log
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

# Test using a custom hostname
rm $LOGFILE
TEST="Ping with custom hostname"
../cronitor ping d3x0c1 --run --hostname customHostnameForTest --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q "hostname=customHostnameForTest" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

rm $LOGFILE
TEST="Ping with message"
MSG="messagewithoutspaces"
../cronitor ping d3x0c1 --run --msg "$MSG" --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q "$MSG" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

# Test with a ping api key
rm $LOGFILE
TEST="Ping with ping api key"
KEY="XXXXXXXXXX"
../cronitor ping d3x0c1 --run --ping-api-key $KEY --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q $KEY $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

# Production integration test
rm $LOGFILE
TEST="Ping integration test"
MSG=`date`
../cronitor ping 44oI2n --run --msg "$MSG" --log $LOGFILE && sleep 1
if ../cronitor activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi


#################
# EXEC TESTS
#################


rm $LOGFILE
TEST="Exec runs command check"
../cronitor --log $LOGFILE exec d3x0c1 ./write-to-log-success.sh $LOGFILE "$TEST"
if grep -q "$TEST" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

rm $LOGFILE
TEST="Exec runs command with complex args"
../cronitor --log $LOGFILE exec d3x0c1 ./success.sh "arg with space" anotherArg
if grep -q "arg with space" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi


rm $LOGFILE
TEST="Exec sends complete ping on success"
../cronitor --log $LOGFILE exec d3x0c1 true
if grep -q "Sending ping https://cronitor.link/d3x0c1/complete" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

rm $LOGFILE
TEST="Exec sends fail ping on failure"
../cronitor --log $LOGFILE exec d3x0c1 false
if grep -q "Sending ping https://cronitor.link/d3x0c1/fail" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

# Production integration test
rm $LOGFILE
TEST="Exec integration test"
MSG=`date`
../cronitor --log $LOGFILE exec 44oI2n echo "$MSG" && sleep 1
if ../cronitor activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi


#################
# STATUS TESTS
#################

# Production integration tests
rm $LOGFILE
TEST="Status integration test without filter"
if ../cronitor status --log $LOGFILE | grep -q "Pass"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

rm $LOGFILE
TEST="Status integration test with filter"
if ../cronitor status 44oI2n --log $LOGFILE | grep -q "Pass"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

#################
# ACTIVITY TESTS
#################

# Activity integration tests
rm $LOGFILE
TEST="Activity integration test without filter"
if ../cronitor activity 44oI2n --log $LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

rm $LOGFILE
TEST="Activity integration test with only pings filter"
if ../cronitor activity 44oI2n --only pings --log $LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi

rm $LOGFILE
TEST="Activity integration test with only alerts filter"
if ../cronitor activity 44oI2n --only alerts --log $LOGFILE | grep -q "No alert history for this monitor"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. Failed"
fi
