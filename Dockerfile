FROM golang:alpine
WORKDIR /app
COPY . /app/
RUN go mod tidy && go mod download && touch .env && go build main.go
ENTRYPOINT [ "./main" ]
