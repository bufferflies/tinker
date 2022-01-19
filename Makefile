IMAGE="pingcap/tinker"
VERSION="master"
make:
	go build .

docker-build:
	docker build -t ${IMAGE}:${VERSION} . --no-cache

docker-push: docker-build
	docker push ${IMAGE}:${VERSION}