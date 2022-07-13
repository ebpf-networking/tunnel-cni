FROM golang:alpine
WORKDIR /go/src/github.com/tunnel-cni
COPY tunnel-bin/go.mod ./
COPY tunnel-bin/*.go ./
RUN go build -o tunnel-cni

FROM golang:alpine
WORKDIR /go/src/github.com/tunnel
COPY go.mod ./
RUN go mod download
COPY *.go ./
RUN go build -o setup_tunnel

FROM golang:alpine
WORKDIR /app
COPY --from=0 /go/src/github.com/tunnel-cni/tunnel-cni /tunnel-cni
COPY --from=1 /go/src/github.com/tunnel/setup_tunnel /setup_tunnel
COPY bridge /bridge
RUN chmod +x /bridge
RUN apk update
RUN apk add --no-cache curl jq iptables iproute2
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl \
    && chmod +x kubectl \
    && mv kubectl /usr/local/bin/kubectl
CMD [ "/setup_tunnel" ]
