SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

if [ -z "$1" ]
  then echo "Usage: release-stable.sh <version number>" && exit 1
fi

if [ -z "$CRONITORCLI_EQUINOX_TOKEN" ]
  then echo "Usage: requires CRONITORCLI_EQUINOX_TOKEN env variable" && exit 1
fi

equinox publish \
 --token=$CRONITORCLI_EQUINOX_TOKEN \
 --app="app_itoJoCoW8dr" \
 --channel="stable" \
 --release="$1"
