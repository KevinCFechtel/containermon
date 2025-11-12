go build  -o ./build/containermon -v -tags "exclude_graphdriver_devicemapper exclude_graphdriver_btrfs" containermon.go
docker buildx create --name MultiBuilder --use
docker buildx build --platform=linux/amd64 -f "Dockerfile" -t kevincfechtel/containermon:latest --push "." 
docker buildx rm MultiBuilder
rm ./build/containermon