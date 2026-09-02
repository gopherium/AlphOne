# syntax=docker/dockerfile:1

# Build the single-page app.
FROM node:26-slim AS frontend
RUN npm install -g pnpm@12.2.1
WORKDIR /app
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json ./
COPY frontend ./frontend
COPY sdk ./sdk
COPY plugins ./plugins
RUN pnpm install --frozen-lockfile
RUN pnpm --filter @alphone/frontend build

# Build the statically linked server binary.
FROM golang:1.27 AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /alphone ./cmd/alphone

# Assemble the runtime image: the binary plus the built SPA.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /alphone /alphone
COPY --from=frontend /app/frontend/dist /web
ENV ALPHONE_WEB_DIR=/web
ENV ALPHONE_ADDR=0.0.0.0:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/alphone"]
