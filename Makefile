IMAGE="pingcap/tikv"
VERSION="master"
make:
	go build .

docker:
	docker build -t $IMAGE:$VERSION .