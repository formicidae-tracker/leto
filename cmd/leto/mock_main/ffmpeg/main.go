package main

import (
	"fmt"
	"io"
	"log"
	"os"
)

func main() {
	if err := execute(); err != nil {
		log.Fatalf("unhandled error: %s", err)
	}
}

func execute() error {
	output := false

	for _, a := range os.Args {
		if a == "-version" {
			printVersion()
			return nil
		}
		if a == "-" {
			output = true
		}
	}

	if output == true {
		return copyStdinToStdout()
	}

	return discardStdin()
}

func copyStdinToStdout() error {
	log.Printf("copying to stdout")
	defer log.Printf("done")
	buffer := make([]byte, 360*270*3)
	for {
		_, err := os.Stdin.Read(buffer)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			log.Printf("got error: %s", err)
			return err
		}
		log.Printf("read %d bytes", len(buffer))
		// only copy part of data, as encoding dicreases the size by a lot
		os.Stdout.Write(buffer[:512])
	}
}

func discardStdin() error {
	log.Printf("discarding to stdin")
	defer log.Printf("done")

	devnull, err := os.Create(os.DevNull)
	if err != nil {
		return err
	}
	_, err = io.Copy(devnull, os.Stdin)
	if err != nil {
		log.Printf("got error: %s", err)
	}
	return err
}

func printVersion() {
	fmt.Println(`ffmpeg version 4.4.2-0ubuntu0.22.04.1 Copyright (c) 2000-2021 the FFmpeg developers
built with gcc 11 (Ubuntu 11.2.0-19ubuntu1)
configuration: --prefix=/usr --extra-version=0ubuntu0.22.04.1 --toolchain=hardened --libdir=/usr/lib/x86_64-linux-gnu --incdir=/usr/include/x86_64-linux-gnu --arch=amd64 --enable-gpl --disable-stripping --enable-gnutls --enable-ladspa --enable-libaom --enable-libass --enable-libbluray --enable-libbs2b --enable-libcaca --enable-libcdio --enable-libcodec2 --enable-libdav1d --enable-libflite --enable-libfontconfig --enable-libfreetype --enable-libfribidi --enable-libgme --enable-libgsm --enable-libjack --enable-libmp3lame --enable-libmysofa --enable-libopenjpeg --enable-libopenmpt --enable-libopus --enable-libpulse --enable-librabbitmq --enable-librubberband --enable-libshine --enable-libsnappy --enable-libsoxr --enable-libspeex --enable-libsrt --enable-libssh --enable-libtheora --enable-libtwolame --enable-libvidstab --enable-libvorbis --enable-libvpx --enable-libwebp --enable-libx265 --enable-libxml2 --enable-libxvid --enable-libzimg --enable-libzmq --enable-libzvbi --enable-lv2 --enable-omx --enable-openal --enable-opencl --enable-opengl --enable-sdl2 --enable-pocketsphinx --enable-librsvg --enable-libmfx --enable-libdc1394 --enable-libdrm --enable-libiec61883 --enable-chromaprint --enable-frei0r --enable-libx264 --enable-shared
libavutil      56. 70.100 / 56. 70.100
libavcodec     58.134.100 / 58.134.100
libavformat    58. 76.100 / 58. 76.100
libavdevice    58. 13.100 / 58. 13.100
libavfilter     7.110.100 /  7.110.100
libswscale      5.  9.100 /  5.  9.100
libswresample   3.  9.100 /  3.  9.100
libpostproc    55.  9.100 / 55.  9.100`)
}
