FROM golang:1.19 as builder
COPY . /app
WORKDIR /app
RUN CGO_ENABLED=0 go build -o /dns-resolve-exporter

FROM gcr.io/distroless/static
COPY --from=builder /dns-resolve-exporter /bin/dns-resolve-exporter
ENTRYPOINT ["/bin/dns-resolve-exporter"]
