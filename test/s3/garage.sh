#!/usr/bin/env bash
set -euo pipefail

container="${1:?usage: test/s3/garage.sh <the Garage container>}"
bucket="${ALPHONE_TEST_S3_BUCKET:?set ALPHONE_TEST_S3_BUCKET, see .env.example}"
key_id="${ALPHONE_TEST_S3_ACCESS_KEY_ID:?set ALPHONE_TEST_S3_ACCESS_KEY_ID, see .env.example}"
secret="${ALPHONE_TEST_S3_SECRET_ACCESS_KEY:?set ALPHONE_TEST_S3_SECRET_ACCESS_KEY, see .env.example}"

garage() {
	docker exec -e RUST_LOG=warn "$container" /garage "$@"
}

for _ in $(seq 1 30); do
	garage status >/dev/null 2>&1 && break
	sleep 1
done

status="$(garage status)"
if [[ "$status" == *"NO ROLE ASSIGNED"* ]]; then
	node="$(garage node id -q)"
	garage layout assign -z dc1 -c 1G "${node%%@*}" >/dev/null
	garage layout apply --version 1 >/dev/null
fi
garage bucket info "$bucket" >/dev/null 2>&1 || garage bucket create "$bucket" >/dev/null
garage key info "$key_id" >/dev/null 2>&1 || garage key import --yes -n alphone-tests "$key_id" "$secret" >/dev/null
garage bucket allow --read --write "$bucket" --key "$key_id" >/dev/null
echo "Garage in $container serves the bucket $bucket to the key $key_id"
