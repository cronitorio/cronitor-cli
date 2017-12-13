#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

if [ "$1" = "--use-dev" ]
    then
        CRONITOR_ARGS="--use-dev"
        HOSTNAME="http://dev.cronitor.io"
    else
        CRONITOR_ARGS=""
        HOSTNAME="${HOSTNAME}"
fi

LOGFILE="/tmp/test-build.log"

# First, build fresh
cd ../
go build && echo "Build created.. OK"
cd - > /dev/null

#################
# CONFIGURE TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Configure uses log file from env var"
CRONITOR_LOG=$LOGFILE ../cronitor $CRONITOR_ARGS ping d3x0c1 --run
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Configure uses hostname from env var"
CRONITOR_HOSTNAME=myHost ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $LOGFILE
if grep -q "&host=myHost" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Configure uses hostname from arg not env var"
CRONITOR_HOSTNAME=myHost ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $LOGFILE --hostname otherHost
if grep -q "&host=otherHost" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Configure uses ping api key from env var"
CRONITOR_PING_API_KEY=123 ../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $LOGFILE
if grep -q "&auth_key=123" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# PING TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Ping without args"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --log $LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping with custom hostname"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --hostname customHostnameForTest --log $LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE && grep -q "host=customHostnameForTest" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping with message"
MSG="messagewithoutspaces"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --msg "$MSG" --log $LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE && grep -q "$MSG" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping with ping api key"
KEY="XXXXXXXXXX"
../cronitor $CRONITOR_ARGS ping d3x0c1 --run --ping-api-key $KEY --log $LOGFILE
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/run" $LOGFILE && grep -q "auth_key=${KEY}" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping integration test"
MSG=`date`
../cronitor $CRONITOR_ARGS ping 44oI2n --run --msg "$MSG" --log $LOGFILE && sleep 1
if ../cronitor $CRONITOR_ARGS activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# EXEC TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Exec runs command check"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./write-to-log-success.sh $LOGFILE "$TEST" > /dev/null
if grep -q "$TEST" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec runs command with complex args"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./success.sh "arg with space" anotherArg > /dev/null
if grep -q "arg with space" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


rm -f $LOGFILE
TEST="Exec sends complete ping on success"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 true > /dev/null
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/complete" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends fail ping on failure"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 false > /dev/null
if grep -q "Sending ping ${HOSTNAME}/d3x0c1/fail" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends status code on complete ping"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./fail.sh > /dev/null
if grep -q "&status_code=123" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends run timestamp as complete ping tag"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 true > /dev/null
if grep -q "&tag=1" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends duration with complete ping"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 sleep 1 > /dev/null
if grep -q "&duration=1." $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends stdout with complete ping"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./success.sh xyz > /dev/null
if grep "&msg=" $LOGFILE | grep -q "xyz"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec does not send stdout when suppressed"
../cronitor $CRONITOR_ARGS --log $LOGFILE exec --no-stdout d3x0c1 ./success.sh xyz > /dev/null
if grep "&msg=" $LOGFILE | grep -q "xyz"
    then echo "${TEST}.. FAIL"
    else echo "${TEST}.. OK"
fi

rm -f $LOGFILE
TEST="Exec passes stdout through to caller"
if ../cronitor $CRONITOR_ARGS --log $LOGFILE exec d3x0c1 ./success.sh xyz | grep -q xyz
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

# Production integration test
rm -f $LOGFILE
TEST="Exec integration test"
MSG=`date`
../cronitor $CRONITOR_ARGS --log $LOGFILE exec 44oI2n echo "$MSG" > /dev/null && sleep 1
if ../cronitor $CRONITOR_ARGS activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# STATUS TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Status integration test without filter"
if ../cronitor $CRONITOR_ARGS status --log $LOGFILE | grep -q "Pass"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Status integration test with filter"
if ../cronitor $CRONITOR_ARGS status 44oI2n --log $LOGFILE | grep -q "Pass"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Status integration test with bad monitor code"
if ../cronitor $CRONITOR_ARGS status asdfgh --log $LOGFILE 2>&1 | grep -q "404"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

#################
# ACTIVITY TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Activity integration test without filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --log $LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Activity integration test with only pings filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --only pings --log $LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Activity integration test with only alerts filter"
if ../cronitor $CRONITOR_ARGS activity 44oI2n --only alerts --log $LOGFILE | grep -q "No alert history for this monitor"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

#################
# DISCOVER TESTS
#################
echo ""
API_KEY="53b6c114717140cf896899060bcc9d7e"

rm -f $LOGFILE
TEST="Discover reads file and sends PUT"
../cronitor $CRONITOR_ARGS discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --log $LOGFILE > /dev/null
if grep -q "Request" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover parses reponse and rewrites crontab"
if ../cronitor $CRONITOR_ARGS discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e| grep "slave_status.sh" | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover rewrites crontab in place"
TMPFILE="/tmp/crontab.txt"
cp ../fixtures/crontab.txt $TMPFILE
../cronitor $CRONITOR_ARGS discover $TMPFILE -k 53b6c114717140cf896899060bcc9d7e --save > /dev/null
if grep "slave_status.sh" $TMPFILE | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
rm -f $TMPFILE

TEST="Discover adds auto-discover"
if ../cronitor $CRONITOR_ARGS discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | grep "cronitor" | grep -q "discover"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover does not add auto-discover when suppressed"
if ../cronitor $CRONITOR_ARGS discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --no-auto-discover | grep -q "cronitor discover"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
    else echo "${TEST}.. OK"
fi

TEST="Discover adds no-stdout flag when supplied"
if ../cronitor $CRONITOR_ARGS discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --no-stdout | grep "cronitor exec" | grep -q "no-stdout"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
    else echo "${TEST}.. OK"
fi

echo ""
