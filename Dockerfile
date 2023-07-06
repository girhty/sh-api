FROM golang:alpine
WORKDIR /app
COPY . .
RUN go mod tidy && go mod download && go build main.go
COPY . .
EXPOSE 3000
CMD ./main