CLI_LOGFILE="/tmp/test-build.log"
CLI_LOGFILE_ALTERNATE="/tmp/test-build-alternate.log"
#CLI_CONFIGFILE="/etc/cronitor/cronitor.json"
CLI_CONFIGFILE="/tmp/cronitor.json"
CLI_CONFIGFILE_ALTERNATE="/tmp/test-build-config.json"
CLI_ACTUAL_API_KEY="cb54ac4fd16142469f2d84fc1bbebd84"
CLI_CRONTAB_TEMP="/tmp/crontab"
CLI_USERNAME=`whoami`

if [ "$1" = "--use-dev" ]
    then
        CRONITOR_ARGS="--use-dev"
        HOSTNAME="http://localhost:8000"
    else
        CRONITOR_ARGS=""
        HOSTNAME="https://cronitor.link"
fi

sudo ../cronitor configure -k "$CLI_ACTUAL_API_KEY" >/dev/null 2>/dev/null