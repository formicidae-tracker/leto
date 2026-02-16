FROM golang:1.24-trixie AS build

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

WORKDIR /app/cmd/leto

RUN go build

FROM ghcr.io/formicidae-tracker/artemis:0.5.0-rc1

RUN apt-get update && apt-get install -y vainfo gstreamer1.0-tools

COPY --from=build /app/cmd/leto/leto /usr/local/bin/leto

WORKDIR /app

CMD [ "/usr/local/bin/leto" ]
