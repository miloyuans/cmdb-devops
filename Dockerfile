FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /out/cmdb-devops ./cmd/cmdb-devops

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/cmdb-devops /app/cmdb-devops
COPY web /app/web
EXPOSE 8080
CMD ["/app/cmdb-devops"]
