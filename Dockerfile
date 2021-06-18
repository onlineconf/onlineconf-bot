FROM golang

WORKDIR /go/src/github.com/onlineconf/onlineconf-myteam-bot

COPY go.* ./
RUN go mod download

COPY *.go ./
RUN go build

FROM gcr.io/distroless/base

COPY --from=0 /go/src/github.com/onlineconf/onlineconf-myteam-bot/onlineconf-myteam-bot /usr/local/bin/onlineconf-myteam-bot

ENTRYPOINT ["onlineconf-myteam-bot"]
