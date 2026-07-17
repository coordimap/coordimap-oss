FROM golang:1.26 AS build-env

RUN apt-get update && apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*

# Add source code
ADD . /src
WORKDIR /src


# Configure git for private repositories
ARG GIT_TOKEN
ENV GOPRIVATE=dev.azure.com
RUN echo ${GIT_TOKEN}
RUN git config --global url."https://${GIT_TOKEN}@dev.azure.com/bloopi/bloopi/_git/shared_models".insteadOf "https://dev.azure.com/bloopi/bloopi/_git/shared_models"
RUN go env -w GOPRIVATE=dev.azure.com



# Build the final Go binary
RUN CGO_ENABLED=0 go build -a -o /coordimap-local ./cmd/coordimap-local

# --- Final Stage ---
FROM alpine:3.21.6

COPY --from=build-env /coordimap-local /coordimap-local

RUN addgroup -S coordimap && adduser -S coordimap -G coordimap
USER coordimap

CMD ["/coordimap-local"]
