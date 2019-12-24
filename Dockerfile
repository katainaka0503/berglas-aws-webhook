FROM golang:1.13 as builder
ADD . /app
WORKDIR /app
RUN go build -o berglas-aws-webhook

FROM alpine:3.11
COPY --from=builder /app/berglas-aws-webhook usr/local/bin/berglas-aws-webhook
CMD ["berglas-aws-webhook"]
