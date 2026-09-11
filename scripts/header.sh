#!/bin/bash

ORG_ID="$1"
USER_NAME="$2"
ACCOUNT_ID="$3"
IS_INTERNAL="$4"

function print_out_usage {
  cat <<EOF
Usage: ./scripts/header.sh <org_id> [user_name] [account_id] [is_internal]
EOF
}

function error {
  print_out_usage >&2
  printf "error: %s\n" "$*" >&2
  exit 1
}

[ "${ORG_ID}" != "" ] || error "ORG_ID is required and cannot be empty"

if [ "${ACCOUNT_ID}" == "" ]; then
  ACCOUNT_ID=$ORG_ID
fi

if [ "${USER_NAME}" == "" ]; then
  USER_NAME="snapUser"
fi

if [ "${IS_INTERNAL}" == "" ]; then
  IS_INTERNAL=false
fi

case "${IS_INTERNAL}" in
true | false)
  ;;
*)
  error "IS_INTERNAL must be true or false"
  ;;
esac

case "$(uname -s)" in
"Darwin")
  ENC="$(echo "{\"identity\":{\"org_id\":\"${ORG_ID}\", \"type\":\"User\",\"user\":{\"username\":\"${USER_NAME}\",\"user_id\":\"${USER_NAME}\",\"is_internal\":${IS_INTERNAL}},\"account_number\":\"${ACCOUNT_ID}\",\"internal\":{\"org_id\":\"${ORG_ID}\"}}}" | base64 -b 0)"
  ;;

"Linux" | *)
  ENC="$(echo "{\"identity\":{\"org_id\":\"${ORG_ID}\", \"type\":\"User\",\"user\":{\"username\":\"${USER_NAME}\",\"user_id\":\"${USER_NAME}\",\"is_internal\":${IS_INTERNAL}},\"account_number\":\"${ACCOUNT_ID}\",\"internal\":{\"org_id\":\"${ORG_ID}\"}}}" | base64 -w0)"
  ;;
esac

echo "x-rh-identity: $ENC"
