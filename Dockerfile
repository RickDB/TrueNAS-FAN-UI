FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY web ./web
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/truenas-fan-ui .

FROM alpine:3.21
RUN addgroup -S fanui && adduser -S -G fanui fanui
COPY --from=build /out/truenas-fan-ui /usr/local/bin/truenas-fan-ui
COPY entrypoint.sh /usr/local/bin/fan-ui-entrypoint
RUN chmod +x /usr/local/bin/fan-ui-entrypoint
EXPOSE 8080
# Must run as root to write host hwmon sysfs nodes.
USER 0
ENTRYPOINT ["/usr/local/bin/fan-ui-entrypoint"]
