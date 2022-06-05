#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

# First, build fresh
cd ../
go build && echo "Build created.. OK"
cd - > /dev/null

./test-activity.sh
./test-configure.sh
./test-discover.sh
./test-exec.sh
./test-list.sh
./test-ping.sh
./test-status.sh
