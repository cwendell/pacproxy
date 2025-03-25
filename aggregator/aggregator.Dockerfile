FROM golang:1.24.1-alpine

WORKDIR /app

COPY ./go.mod ./go.sum ./
RUN go mod download

COPY aggregator/aggregator.go ./
COPY shared ./shared/

RUN go build -o /aggregator 

CMD ["/aggregator"]