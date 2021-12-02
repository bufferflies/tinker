FROM golang:1.16 as tinker
ENV GO111MODULE=on
RUN mkdir -p /go/src/github.com/pingcap/go-tinker
WORKDIR /go/src/github.com/pingcap/go-tinker
RUN git clone https://github.com/bufferflies/tinker.git  .
RUN go build .


FROM golang:1.16
COPY --from=tinker /go/src/github.com/pingcap/go-tinker/* /bin/
ENV PATH="$PATH:/bin"
WORKDIR /go/src