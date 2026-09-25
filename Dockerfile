# syntax=docker/dockerfile:1

# Runs a history download with a token file mounted from the host:
#
#   docker build -t costco-cli .
#   docker run --rm \
#     -v ~/.costco/tokens.json:/secrets/tokens.json \
#     -v ~/costco:/data \
#     costco-cli
#
# Sign in on the host first ('costco-cli -cmd login'); the container cannot.

FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG COMMIT=""
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.commit=${COMMIT}" -o /costco-cli ./cmd/costco-cli

# The static distroless image carries CA certificates for HTTPS and nothing
# else: no shell, no package manager.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /costco-cli /usr/local/bin/costco-cli
ENV COSTCO_TOKEN_FILE=/secrets/tokens.json
WORKDIR /data
# -non-interactive: nobody can answer a sign-in prompt here, so an expired token
# fails straight away with instructions instead of waiting on input.
ENTRYPOINT ["costco-cli", "-non-interactive", "-out", "/data/costco-history"]
