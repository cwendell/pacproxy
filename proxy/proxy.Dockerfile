FROM golang:1.24.1-alpine

WORKDIR /app

COPY ./go.mod ./go.sum ./
RUN go mod download

COPY proxy/proxy.go ./
copy proxy/producer_manager.go ./
COPY shared ./shared/

RUN go build -o /proxy 

EXPOSE 8080

CMD ["/proxy"]