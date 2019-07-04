FROM golang:1.11

WORKDIR /app

COPY . .

RUN go install -v ./cmd/...

EXPOSE 8080
ENTRYPOINT ["courier"]