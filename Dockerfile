FROM golang:1.8.1

WORKDIR /go/src/app

COPY . .

RUN go-wrapper download
RUN go-wrapper install

EXPOSE 8080

CMD ["go-wrapper", "run"]
