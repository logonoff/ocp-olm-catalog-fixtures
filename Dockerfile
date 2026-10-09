FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY cmd/controller ./cmd/controller
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /controller ./cmd/controller

FROM scratch
COPY --from=build /controller /controller
USER 65532:65532
EXPOSE 8000
ENTRYPOINT ["/controller"]
