#!/bin/bash

DN="$1"


function print_out_usage {
	cat <<EOF
Usage: ./scripts/turnpike_header.sh DN_VALUE
EOF
}

function error {
	local err=$?
	print_out_usage >&2
	printf "error: %s\n" "$*" >&2
	exit $err
}

[ "${DN}" != "" ] || error "DN is required and cannot be empty"

case "$( uname -s )" in
"Darwin" )
  ENC="$(echo "{\"identity\":{\"x509\":{\"subject_dn\":\"${DN}\"}}}" | base64 -b 0)"
;;

"Linux" | *)
  ENC="$(echo "{\"identity\":{\"x509\":{\"subject_dn\":\"${DN}\"}}}" | base64 -w0)"
;;
esac

echo "x-rh-identity: $ENC"
