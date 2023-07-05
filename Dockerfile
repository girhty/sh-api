FROM golang:alpine
WORKDIR /app/api
COPY ./api .
RUN go build main.go
CMD ./main