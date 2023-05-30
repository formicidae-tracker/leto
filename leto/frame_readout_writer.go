package main

import (
	"compress/gzip"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/golang/protobuf/proto"
)

type FrameReadoutFileWriter struct {
	period                         time.Duration
	basename                       string
	lastname, lastUncompressedName string
	file, uncompressed             *os.File
	gzip                           *gzip.Writer
	logger                         *log.Logger
	quit                           chan struct{}
}

func NewFrameReadoutWriter(filepath string) (*FrameReadoutFileWriter, error) {

	return &FrameReadoutFileWriter{
		period:   2 * time.Hour,
		basename: filepath,
		quit:     make(chan struct{}),
		logger:   NewLogger(fmt.Sprintf("file/%s", filepath)),
	}, nil

}

func (w *FrameReadoutFileWriter) openFile(filename, filenameUncompressed string, width, height int32) error {
	var err error
	w.file, err = os.Create(filename)
	if err != nil {
		return err
	}
	w.uncompressed, err = os.Create(filenameUncompressed)
	if err != nil {
		return err
	}

	w.gzip = gzip.NewWriter(w.file)

	header := &hermes.Header{
		Type: hermes.Header_File,
		Version: &hermes.Version{
			Vmajor: 0,
			Vminor: 2,
		},
		Width:  width,
		Height: height,
	}
	if len(w.lastname) > 0 {
		header.Previous = filepath.Base(w.lastname)
	}

	w.lastname = filename
	w.lastUncompressedName = filenameUncompressed

	b := proto.NewBuffer(nil)
	err = b.EncodeMessage(header)
	if err != nil {
		return err
	}

	_, err = w.gzip.Write(b.Bytes())
	if err != nil {
		return err
	}
	_, err = w.uncompressed.Write(b.Bytes())
	log.Printf("Writing to file '%s' and '%s'", filename, filenameUncompressed)
	return err
}

func (w *FrameReadoutFileWriter) closeFiles(nextFile string) {
	footer := &hermes.Footer{}
	if len(nextFile) > 0 {
		footer.Next = filepath.Base(nextFile)
	}

	line := &hermes.FileLine{
		Footer: footer,
	}

	if w.gzip != nil && w.file != nil {
		b := proto.NewBuffer(nil)
		if err := b.EncodeMessage(line); err != nil {
			w.logger.Printf("Could not encode footer: %s", err)
		} else {
			if _, err := w.gzip.Write(b.Bytes()); err != nil {
				w.logger.Printf("Could not write footer: %s", err)
			}
			if _, err := w.uncompressed.Write(b.Bytes()); err != nil {
				w.logger.Printf("Could not write uncompressed footer: %s", err)
			}
		}
	}

	if w.gzip != nil {
		if err := w.gzip.Close(); err != nil {
			w.logger.Printf("could not close gzipper: %s", err)
		}
		w.gzip = nil
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			w.logger.Printf("could not close '%s': %s", w.lastname, err)
		}
		w.file = nil
	}
	if w.uncompressed != nil {
		if err := w.uncompressed.Close(); err != nil {
			w.logger.Printf("could not close uncompressed file '%s': %s", w.lastUncompressedName, err)
		}
		if err := os.RemoveAll(w.lastUncompressedName); err != nil {
			w.logger.Printf("could not remove last uncompressed segment '%s': %s", w.lastUncompressedName, err)
		}
	}
}

func (w *FrameReadoutFileWriter) Close() error {
	close(w.quit)
	w.closeFiles("")
	return nil
}

func (w *FrameReadoutFileWriter) WriteAll(readout <-chan *hermes.FrameReadout) {
	ticker := time.NewTicker(w.period)
	defer func() {
		ticker.Stop()
		w.closeFiles("")
	}()

	closeNext := false
	nextName, _, err := FilenameWithoutOverwrite(w.basename)
	if err != nil {
		w.logger.Printf("Could not find unique name: %s", err)
		return
	}

	for {
		select {
		case <-ticker.C:
			closeNext = true
		case <-w.quit:
			return
		case r, ok := <-readout:
			if ok == false {
				return
			}
			if w.file == nil {
				err := w.openFile(nextName, "uncompressed-"+nextName, r.Width, r.Height)
				if err != nil {
					w.logger.Printf("Could not create file '%s': %s", nextName, err)
					return
				}
			}

			// makes a semi-shallow copy to strip away unucessary
			// information. Most of the data is the list of ants and
			// we just do a shallow copy of the slice. The other
			// embedded field could be modified freely
			toWrite := *r

			// removes unucessary information on a per-frame basis. It
			// is concurrently safe since we are not modifying a
			// pointed field.
			toWrite.ProducerUuid = ""
			toWrite.Quads = 0
			toWrite.Width = 0
			toWrite.Height = 0

			b := proto.NewBuffer(nil)
			line := &hermes.FileLine{
				Readout: &toWrite,
			}
			if err := b.EncodeMessage(line); err != nil {
				w.logger.Printf("Could not encode message: %s", err)
			}
			_, err := w.gzip.Write(b.Bytes())
			if err != nil {
				w.logger.Printf("Could not write message: %s", err)
				return
			}
			_, err = w.uncompressed.Write(b.Bytes())
			if err != nil {
				w.logger.Printf("Could not write uncompressed message: %s", err)
				return
			}

			if closeNext == false {
				continue
			}
			closeNext = false

			nextName, _, err = FilenameWithoutOverwrite(w.basename)
			if err != nil {
				w.logger.Printf("Could not find unique name: %s", err)
			}
			w.closeFiles(nextName)
		}
	}
}
