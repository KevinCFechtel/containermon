docker buildx create --name MultiBuilder --use
docker buildx build --platform=linux/amd64 -f "Dockerfile" -t kevincfechtel/containermon:latest --push "." 
docker buildx rm MultiBuilder
