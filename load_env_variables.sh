#!/bin/bash

echo "Load environment variables..."


export $(xargs <.env)

# if [ -f .env ]; then
#     export $(cat .env | xargs)
# else
#     echo "No .env file found; using environment variables from GitHub Secrets..."
# fi

# set -a
# [ -f .env ] && . .env
# set +a
# echo $PATH
# echo $POSTGRES_USER

export DB_HOST=my-postgres
export DATABASE_URI=postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@$DB_HOST/$POSTGRES_DB?sslmode=disable
export TMP_DB_PORT=5433


echo $DATABASE_URI
