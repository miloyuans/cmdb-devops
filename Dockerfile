FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata git
COPY . .
RUN set -eux; \
    if [ -f /src/go.mod ]; then root=/src; \
    elif [ -f /src/cmdb-devops/go.mod ]; then root=/src/cmdb-devops; \
    else echo "go.mod not found. Check docker build context."; find /src -maxdepth 3 -type d | sort; exit 1; fi; \
    cd "$root"; \
    if [ ! -d ./cmd/cmdb-devops ]; then echo "cmd/cmdb-devops not found under $root"; find "$root" -maxdepth 4 -type d | sort; exit 1; fi; \
    go env -w GOPROXY=https://proxy.golang.org,direct; \
    go mod tidy; \
    go mod download; \
    CGO_ENABLED=0 GOOS=linux go build -mod=mod -trimpath -ldflags="-s -w" -o /out/cmdb-devops ./cmd/cmdb-devops; \
    cp -a "$root/web" /out/web

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -s /sbin/nologin appuser
COPY --from=build /out/cmdb-devops /app/cmdb-devops
COPY --from=build /out/web /app/web
EXPOSE 8080
USER appuser
CMD ["/app/cmdb-devops"]
