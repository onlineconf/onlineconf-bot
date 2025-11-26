ARG IMAGE_GOLANG=golang:bullseye
FROM ${IMAGE_GOLANG}

WORKDIR /go/src/github.com/onlineconf/onlineconf-bot

COPY go.* ./
RUN go mod download -x

COPY *.go ./
COPY cmd/ ./cmd/
RUN CGO_ENABLED=0 go build -x -o ./bin/ ./cmd/*/

FROM gcr.io/distroless/base

COPY --from=0 \
	/go/src/github.com/onlineconf/onlineconf-bot/bin/* \
	/usr/bin/

ENTRYPOINT ["onlineconf-mattermost-bot"]
