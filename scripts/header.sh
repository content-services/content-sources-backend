#!/bin/bash

ORG_ID="$1"
USER_NAME="$2"
ACCOUNT_ID="$3"


function print_out_usage {
	cat <<EOF
Usage: ./scripts/header.sh <org_id> [user_name]
EOF
}

function error {
	local err=$?
	print_out_usage >&2
	printf "error: %s\n" "$*" >&2
	exit $err
}

[ "${ORG_ID}" != "" ] || error "ORG_ID is required and cannot be empty"

if [ "${ACCOUNT_ID}" == "" ]; then
  ACCOUNT_ID=$ORG_ID
fi

if [ "${USER_NAME}" == "" ]; then
  USER_NAME="snapUser"
fi

case "$( uname -s )" in
"Darwin" )
  ENC="$(echo "{\"identity\":{\"type\":\"User\",\"user\":{\"username\":\"${USER_NAME}\"},\"account_number\":\"${ACCOUNT_ID}\",\"internal\":{\"org_id\":\"${ORG_ID}\"}}}" | base64 -b 0)"
;;

"Linux" | *)
  ENC="$(echo "{\"identity\":{\"type\":\"User\",\"user\":{\"username\":\"${USER_NAME}\"},\"account_number\":\"${ACCOUNT_ID}\",\"internal\":{\"org_id\":\"${ORG_ID}\"}}}" | base64 -w0)"
;;
esac

echo "x-rh-identity: $ENC"
