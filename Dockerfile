# build stage
FROM golang as builder

WORKDIR /app
ENV GO111MODULE=on \
 SAPNWRFC_HOME="/app/nwrfcsdk" \
 CGO_LDFLAGS="-L /app/nwrfcsdk/lib" \
 CGO_CFLAGS="-I /app/nwrfcsdk/include" \
 LD_LIBRARY_PATH="/app/nwrfcsdk/lib" \
 CGO_CFLAGS_ALLOW="^.*" \
 CGO_LDFLAGS_ALLOW="^.*"

COPY go.mod .
COPY go.sum .

RUN go mod download

# FROM build_base AS server_builder
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build


# final stage
FROM frolvlad/alpine-glibc
RUN apk add libuuid
RUN apk add libstdc++
RUN apk add libstdc++6

ENV SAPNWRFC_HOME="/app/nwrfcsdk" \
 CGO_LDFLAGS="-L /app/nwrfcsdk/lib" \
 CGO_CFLAGS="-I /app/nwrfcsdk/include" \
 LD_LIBRARY_PATH="/app/nwrfcsdk/lib" \
 CGO_CFLAGS_ALLOW="^.*" \
 CGO_LDFLAGS_ALLOW="^.*"

COPY --from=builder /app/nwrfcsdk /app/nwrfcsdk
COPY --from=builder /app/sapnwrfc_exporter /app/sapnwrfc_exporter

EXPOSE 9663
ENTRYPOINT ["/app/sapnwrfc_exporter","web","--port","9663","--config","/app/sapnwrfc_exporter.toml","--timeout","5"]
