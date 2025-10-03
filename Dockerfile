FROM golang:alpine
WORKDIR /app
COPY . .
RUN go mod tidy && go mod download && go build main.go
EXPOSE 8443
ENTRYPOINT [ "./main" ]
