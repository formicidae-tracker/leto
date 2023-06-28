FROM golang:1.20-bullseye as build

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

WORKDIR /app/cmd/leto

RUN go build

FROM ghcr.io/formicidae-tracker/artemis:0.4.5

RUN apt-get update && apt-get install -y ffmpeg

WORKDIR /app

COPY --from=build /app/cmd/leto/leto ./leto

RUN groupadd -g 1000 fort-user

RUN useradd -d /home/fort-user -s /bin/sh -m fort-user -u 1000 -g 1000

USER fort-user

ENV HOME /home/fort-user

ENTRYPOINT [ "./leto" ]
