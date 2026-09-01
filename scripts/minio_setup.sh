#!/bin/bash
# Развертывание бакета Minio для режимов работы мотора и загрузка медиа.
# Использование: ./scripts/minio_setup.sh ../media
set -e

MEDIA_DIR="${1:-../media}"
CONTAINER="ebike_minio_storage"
BUCKET="ebike-motor-media"

docker exec "$CONTAINER" mc alias set myminio http://localhost:9000 root rootpassword
docker exec "$CONTAINER" mc mb --ignore-existing "myminio/$BUCKET"
docker exec "$CONTAINER" mc anonymous set download "myminio/$BUCKET"

for file in "$MEDIA_DIR"/images/*.jpg "$MEDIA_DIR"/videos/*.mp4; do
    [ -e "$file" ] || continue
    name="$(basename "$file")"
    docker cp "$file" "$CONTAINER:/tmp/$name"
    docker exec "$CONTAINER" mc cp "/tmp/$name" "myminio/$BUCKET/$name"
    docker exec "$CONTAINER" rm -f "/tmp/$name"
done

docker exec "$CONTAINER" mc ls "myminio/$BUCKET"
