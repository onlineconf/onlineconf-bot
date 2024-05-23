FROM golang:bullseye

WORKDIR /go/src/github.com/onlineconf/onlineconf-bot

COPY go.* *.go .
COPY cmd/ ./cmd/
RUN go build -o . ./cmd/onlineconf-myteam-bot/ ./cmd/onlineconf-mattermost-bot/

FROM gcr.io/distroless/base

COPY --from=0 /go/src/github.com/onlineconf/onlineconf-bot/onlineconf-myteam-bot \
		      /go/src/github.com/onlineconf/onlineconf-bot/onlineconf-mattermost-bot \
			  /usr/bin/

ENTRYPOINT ["onlineconf-mattermost-bot"]
