#!/bin/bash

echo "Checking for local postgres..."

if brew services list | grep "postgresql\s*started" > /dev/null; then
    echo "OK."
else
    echo "ERROR: local postgres not found!"
    echo
    echo "Please make sure postgres is installed and running locally:"
    echo "  $ brew install postgresql"
    echo "  $ brew services start postgresql"
    exit 1
fi

echo "Resetting local database 'siphon_bundler_test'..."
echo "drop database siphon_bundler_test;" | psql -h localhost postgres
echo "create database siphon_bundler_test;" | psql -h localhost postgres
echo "Done."

export SIPHON_ENV="testing"
export POSTGRES_BUNDLER_ENV_POSTGRES_USER="`whoami`"
export POSTGRES_BUNDLER_PORT_5432_TCP_ADDR="localhost"
export POSTGRES_BUNDLER_ENV_POSTGRES_DB="siphon_bundler_test"
export AWS_ACCESS_KEY_ID="AKIAJOAPGZD2SYWXWBWQ"
export AWS_SECRET_ACCESS_KEY="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"

(cd tests && python -m unittest $@ )
