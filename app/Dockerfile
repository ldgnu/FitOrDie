# syntax=docker/dockerfile:1
# FitOrDie — build multi-stage, binario único, usuario no-root, imagen mínima.
#
# El brief pide Go 1.22+; usamos la imagen oficial 1.27 porque la dependencia
# modernc.org/sqlite v1.61 declara `go 1.26.0` en su go.mod (requisito duro).
FROM golang:1.27-alpine AS build

WORKDIR /src

# Dependencias primero: cachea la capa de módulos aunque cambie el código.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 -> binario estático, corre en distroless/static.
ARG VERSION=dev
ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/fitordie ./cmd/fitordie \
 && /out/fitordie version \
 && mkdir -p /out/data

# --- runtime -------------------------------------------------------------
# distroless/static: sin shell, sin package manager, con usuario nonroot (65532).
FROM gcr.io/distroless/static:nonroot

WORKDIR /app
COPY --from=build /out/fitordie /app/fitordie
# /data nace con dueño nonroot: un volumen nombrado hereda el ownership de la
# imagen en su primera creación, así SQLite puede escribir sin correr como root.
COPY --from=build --chown=65532:65532 /out/data /data

# SQLite vive en /data (volumen). El CSV se embebe en el binario, así que el
# contenedor arranca y se siembra solo la primera vez.
ENV DB_PATH=/data/fitordie.db \
    ADDR=0.0.0.0:3007 \
    TZ_NAME=America/Argentina/Cordoba \
    GOMEMLIMIT=64MiB \
    GOGC=50

USER nonroot:nonroot
EXPOSE 3007

# Healthcheck sin shell: el propio binario consulta /healthz.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/app/fitordie", "health"]

ENTRYPOINT ["/app/fitordie"]
CMD ["serve"]
