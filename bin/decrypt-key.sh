SCRIPT_DIR=$( cd $(dirname $0) ; pwd -P )
cd $SCRIPT_DIR

# private key encrypted using:
# $ openssl aes-256-cbc -a -salt -in equinox.key -out equinox.key.encrypted

openssl aes-256-cbc -d -a -in ../equinox.key.encrypted -out ../equinox.key
