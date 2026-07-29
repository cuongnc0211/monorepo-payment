FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/banksim ./cmd/banksim

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/banksim /usr/local/bin/banksim
EXPOSE 9090
ENTRYPOINT ["banksim"]
