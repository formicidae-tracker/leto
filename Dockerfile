FROM ubuntu:22.04

COPY --from=golang:1.20-bullseye /usr/local/go /usr/local/go/

ENV PATH="/usr/local/go/bin:${PATH}"

RUN apt-get update && apt-get install -y ca-certificates

RUN sed -i 's/htt[p|ps]:\/\/archive.ubuntu.com\/ubuntu\//http:\/\/mirror.infomaniak.ch\/ubuntu/g' /etc/apt/sources.list
RUN sed -i 's/htt[p|ps]:\/\/security.ubuntu.com\/ubuntu\//http:\/\/mirror.infomaniak.ch\/ubuntu/g' /etc/apt/sources.list

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
	build-essential \
	cmake \
	git \
	git-lfs \
	protobuf-compiler \
	libprotobuf-dev \
	libboost-system-dev \
	libopencv-dev \
	libopencv-imgproc-dev \
	libasio-dev \
	libglew-dev \
	libglfw3-dev \
	libeigen3-dev \
	libfontconfig1-dev \
	libfreetype6-dev \
	libgoogle-glog-dev \
	google-mock \
	ffmpeg


WORKDIR /app

RUN git clone https://github.com/formicidae-tracker/artemis

WORKDIR /app/artemis

RUN git checkout v0.4.5

RUN mkdir -p build

WORKDIR /app/artemis/build

RUN cmake ../  \
	-DCMAKE_BUILD_TYPE=RelWithDebInfo \
	-DFORCE_STUB_FRAMEGRABBER_ONLY=On \
	&& make

RUN make install

RUN ldconfig

WORKDIR /app/leto

COPY . .

RUN go mod download

WORKDIR /app/leto/leto

RUN go build

CMD [ "./leto" ]
