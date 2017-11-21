#!/usr/bin/env bash

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

LOGFILE="/tmp/test-build.log"

# First, build fresh
cd ../
go build
cd - > /dev/null

#################
# PING TESTS
#################
echo ""

# Test basic ping
rm -f $LOGFILE
TEST="Ping without args"
../cronitor ping d3x0c1 --run --log /tmp/test-build.log
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

# Test using a custom hostname
rm -f $LOGFILE
TEST="Ping with custom hostname"
../cronitor ping d3x0c1 --run --hostname customHostnameForTest --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q "host=customHostnameForTest" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Ping with message"
MSG="messagewithoutspaces"
../cronitor ping d3x0c1 --run --msg "$MSG" --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q "$MSG" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

# Test with a ping api key
rm -f $LOGFILE
TEST="Ping with ping api key"
KEY="XXXXXXXXXX"
../cronitor ping d3x0c1 --run --ping-api-key $KEY --log $LOGFILE
if grep -q "Sending ping https://cronitor.link/d3x0c1/run" $LOGFILE && grep -q $KEY $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

# Production integration test
rm -f $LOGFILE
TEST="Ping integration test"
MSG=`date`
../cronitor ping 44oI2n --run --msg "$MSG" --log $LOGFILE && sleep 1
if ../cronitor activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# EXEC TESTS
#################
echo ""

rm -f $LOGFILE
TEST="Exec runs command check"
../cronitor --log $LOGFILE exec d3x0c1 ./write-to-log-success.sh $LOGFILE "$TEST"
if grep -q "$TEST" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec runs command with complex args"
../cronitor --log $LOGFILE exec d3x0c1 ./success.sh "arg with space" anotherArg
if grep -q "arg with space" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


rm -f $LOGFILE
TEST="Exec sends complete ping on success"
../cronitor --log $LOGFILE exec d3x0c1 true
if grep -q "Sending ping https://cronitor.link/d3x0c1/complete" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends fail ping on failure"
../cronitor --log $LOGFILE exec d3x0c1 false
if grep -q "Sending ping https://cronitor.link/d3x0c1/fail" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends status code on complete ping"
../cronitor --log $LOGFILE exec d3x0c1 ./fail.sh
if grep -q "status_code=123" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends run timestamp as complete ping tag"
../cronitor --log $LOGFILE exec d3x0c1 true
if grep -q "tag=1" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Exec sends duration with complete ping"
../cronitor --log $LOGFILE exec d3x0c1 sleep 1
if grep -q "duration=1." $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


# Production integration test
rm -f $LOGFILE
TEST="Exec integration test"
MSG=`date`
../cronitor --log $LOGFILE exec 44oI2n echo "$MSG" && sleep 1
if ../cronitor activity 44oI2n | grep -q "$MSG"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi


#################
# STATUS TESTS
#################
echo ""

# Production integration tests
rm -f $LOGFILE
TEST="Status integration test without filter"
if ../cronitor status --log $LOGFILE | grep -q "Pass"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Status integration test with filter"
if ../cronitor status 44oI2n --log $LOGFILE | grep -q "Pass"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Status integration test with bad monitor code"
if ../cronitor status asdfgh --log $LOGFILE 2>&1 | grep -q "404"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

#################
# ACTIVITY TESTS
#################
echo ""

# Activity integration tests
rm -f $LOGFILE
TEST="Activity integration test without filter"
if ../cronitor activity 44oI2n --log $LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Activity integration test with only pings filter"
if ../cronitor activity 44oI2n --only pings --log $LOGFILE | grep -q "monitor_name"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Activity integration test with only alerts filter"
if ../cronitor activity 44oI2n --only alerts --log $LOGFILE | grep -q "No alert history for this monitor"
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
../cronitor discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --log $LOGFILE > /dev/null
if grep -q "Request" $LOGFILE
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

rm -f $LOGFILE
TEST="Discover parses reponse and rewrites crontab"
if ../cronitor discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e| grep "slave_status.sh" | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover rewrites crontab in place"
TMPFILE="/tmp/crontab.txt"
cp ../fixtures/crontab.txt $TMPFILE
../cronitor discover $TMPFILE -k 53b6c114717140cf896899060bcc9d7e --save > /dev/null
if grep "slave_status.sh" $TMPFILE | grep -q "cronitor exec"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi
rm -f $TMPFILE

TEST="Discover adds auto-discover"
if ../cronitor discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e | grep -q "cronitor discover"
    then echo "${TEST}.. OK"
    else echo "${TEST}.. FAIL"
fi

TEST="Discover does not add auto-discover when suppressed"
if ../cronitor discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --no-auto-discover | grep -q "cronitor discover"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
    else echo "${TEST}.. OK"
fi

TEST="Discover adds no-stdout flag when supplied"
if ../cronitor discover ../fixtures/crontab.txt -k 53b6c114717140cf896899060bcc9d7e --no-stdout | grep "cronitor exec" | grep -q "no-stdout"
    then echo "${TEST}.. FAIL"  # Note reversed order here...
    else echo "${TEST}.. OK"
fi

echo ""
