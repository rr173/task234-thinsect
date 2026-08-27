FROM golang:1.26.3-bookworm

ENV CGO_ENABLED=0 \
    GOFLAGS=-mod=mod \
    GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn \
    GOTOOLCHAIN=local

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/thinsect ./cmd/thinsect

WORKDIR /data
ENTRYPOINT ["/out/thinsect"]
CMD ["--smoke-test"]
