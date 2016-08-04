#!/bin/bash
echo "Setting up env variables..."
eval "$(docker-machine env default)"

if [ "$1" = "--psql" ]; then
    docker run -it --link postgres_bundler --rm --env='PGPASSWORD=bundler' postgres:9.4.5 sh -c 'exec psql -h "$POSTGRES_BUNDLER_PORT_5432_TCP_ADDR" -p "$POSTGRES_BUNDLER_PORT_5432_TCP_PORT" -U bundler'
    exit 0
fi

# Note: AWS key below corresponds to the bundler-dev IAM user
COMPOSE_FILE="compose.yml"
TMP_COMPOSE_FILE=".tmp-compose"
cat $COMPOSE_FILE \
    | sed -e 's/$SIPHON_ENV/staging/g' \
    | sed -e 's/$WEB_HOST/local.getsiphon.com/g' \
    | sed -e 's/$RABBITMQ_HOST/local.getsiphon.com/g' \
    | sed -e 's/$RABBITMQ_PORT/5672/g' \
    | sed -e 's/$AWS_ACCESS_KEY_ID/AKIAJ5L4LH4KN7MKKDLA/g' \
    | sed -e 's/$AWS_SECRET_ACCESS_KEY/w8zYYKTjeHtNwJaRQ2cyMnzLOGDpmeh8XWe4QgH7/g' > "${TMP_COMPOSE_FILE}"

# echo "Stopping any running containers..."
# docker-compose -f $TMP_COMPOSE_FILE stop
# docker stop $(docker ps -a -q)
#docker-compose -f $COMPOSE_FILE rm

echo "Building and running containers..."
docker-compose -f $TMP_COMPOSE_FILE build && docker-compose -f $TMP_COMPOSE_FILE up && rm -f $TMP_COMPOSE_FILE
