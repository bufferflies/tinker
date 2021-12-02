IMAGE="pingcap/tikv"
VERSION="master"
make:
	go build .

docker-build:
	docker build -t ${IMAGE}:${VERSION} .

docker-push: docker-build
    docker push ${IMAGE}:${VERSION}