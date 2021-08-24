FROM golang:1.17-alpine AS builder
ENV USER=app
ENV UID=10001
RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    "${USER}"
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -trimpath -o build/ ./cmd/...

FROM scratch
USER app:app
EXPOSE 9001
ENTRYPOINT ["/bin/miniconsole"]
ENV MINIO_ENDPOINT=minio:9000         \
    MINIO_KEY_ID=minioadmin           \
    MINO_ACCESS_KEY=minioadmin        \
    MINIO_SSL="false"                 \
    BIND=:9001                        \
    MAX_OBJECTS="1024"                \
    GIN_MODE=release
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /app/build/ /bin/
CMD ["serve"]