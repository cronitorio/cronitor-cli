SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

if [ -z "$1" ]
  then echo "Usage: release-stable.sh <version number>" && exit 1
fi

equinox publish \
 --token="***REMOVED***" \
 --app="app_itoJoCoW8dr" \
 --channel="stable" \
 --version="$1"
