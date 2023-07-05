package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/formicidae-tracker/hermes"
	"github.com/formicidae-tracker/olympus/pkg/tm"
	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"
)

type HermesFileWriter interface {
	Task
	Incoming() chan<- *hermes.FrameReadout
}

type hermesFileWriter struct {
	period                         time.Duration
	basename                       string
	lastname, lastUncompressedName string
	file, uncompressed             *os.File
	gzip                           *gzip.Writer
	logger                         *logrus.Entry
	incoming                       chan *hermes.FrameReadout
}

func NewFrameReadoutWriter(ctx context.Context, filepath string) (HermesFileWriter, error) {

	return &hermesFileWriter{
		period:   2 * time.Hour,
		basename: filepath,
		logger:   tm.NewLogger("file-writer").WithContext(ctx),
		incoming: make(chan *hermes.FrameReadout, 200),
	}, nil

}

func (w *hermesFileWriter) Incoming() chan<- *hermes.FrameReadout {
	return w.incoming
}

func (w *hermesFileWriter) openFile(filename, filenameUncompressed string, width, height int32) error {
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
	w.logger.WithFields(logrus.Fields{
		"compressed-file":   filename,
		"uncompressed-file": filenameUncompressed,
	}).Info("destination files")
	return err
}

func (w *hermesFileWriter) closeUncompressed() error {
	if w.uncompressed == nil {
		return nil
	}
	defer func() { w.uncompressed = nil }()

	if err := w.uncompressed.Close(); err != nil {
		return fmt.Errorf("could not close uncompressed file '%s': %w", w.lastUncompressedName, err)
	}

	if err := os.RemoveAll(w.lastUncompressedName); err != nil {
		w.logger.WithFields(logrus.Fields{
			"file":  w.lastUncompressedName,
			"error": err,
		}).Error("could not remove last uncompressed segment")
	}

	return nil
}

func (w *hermesFileWriter) closeFile() error {
	if w.file == nil {
		return nil
	}
	defer func() { w.file = nil }()
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("could not close '%s': %w", w.lastname, err)
	}
	return nil
}

func (w *hermesFileWriter) closeGzip() error {
	if w.gzip == nil {
		return nil
	}
	defer func() { w.gzip = nil }()
	if err := w.gzip.Close(); err != nil {
		return fmt.Errorf("could not close gzipper: %w", err)
	}
	return nil
}

func (w *hermesFileWriter) closeFiles(nextFile string) (retError error) {
	defer func() {
		err := w.closeUncompressed()
		if retError == nil {
			retError = err
		} else if err != nil {
			w.logger.WithError(err).Error("additional close error for uncompressed file")
		}
	}()
	defer func() {
		err := w.closeFile()

		if retError == nil {
			retError = err
		} else if err != nil {
			w.logger.WithError(err).Error("additional close error for compressed file")
		}
	}()
	defer func() {
		err := w.closeGzip()
		if retError == nil {
			retError = err
		} else if err != nil {
			w.logger.WithError(err).Error("additional close error for GZIP stream")
		}
	}()

	footer := &hermes.Footer{}
	if len(nextFile) > 0 {
		footer.Next = filepath.Base(nextFile)
	}

	line := &hermes.FileLine{
		Footer: footer,
	}

	if w.gzip == nil || w.file == nil || w.uncompressed == nil {
		return nil
	}

	b := proto.NewBuffer(nil)

	if err := b.EncodeMessage(line); err != nil {
		return fmt.Errorf("could not encode footer: %w", err)
	}

	if _, err := w.gzip.Write(b.Bytes()); err != nil {
		return fmt.Errorf("could not write footer: %w", err)
	}
	if _, err := w.uncompressed.Write(b.Bytes()); err != nil {
		return fmt.Errorf("could not write uncompressed footer: %s", err)
	}
	return nil
}

func (w *hermesFileWriter) uncompressedName(filename string) string {
	base := filepath.Base(filename)
	return filepath.Join(filepath.Dir(filename), "uncompressed-"+base)
}

func (w *hermesFileWriter) writeLine(r *hermes.FrameReadout, nextName string) error {
	if w.file == nil {
		err := w.openFile(nextName, w.uncompressedName(nextName), r.Width, r.Height)
		if err != nil {
			return err
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
		return fmt.Errorf("could not encode message: %w", err)
	}
	_, err := w.gzip.Write(b.Bytes())
	if err != nil {
		return fmt.Errorf("could not write message: %w", err)
	}
	_, err = w.uncompressed.Write(b.Bytes())
	if err != nil {
		return fmt.Errorf("could not write uncompressed message: %w", err)
	}
	return nil
}

func (w *hermesFileWriter) closeAndGetNextName() (string, error) {
	nextName, _, err := FilenameWithoutOverwrite(w.basename)
	if err != nil {
		return "", fmt.Errorf("could not find unique name: %w", err)
	}
	return nextName, w.closeFiles(nextName)
}

func (w *hermesFileWriter) Run() (retError error) {
	ticker := time.NewTicker(w.period)
	defer func() {
		ticker.Stop()
		err := w.closeFiles("")
		if retError == nil {
			retError = err
		}
	}()

	closeNext := false
	nextName, _, err := FilenameWithoutOverwrite(w.basename)
	if err != nil {
		return fmt.Errorf("could not find unique name: %w", err)
	}

	for {
		select {
		case <-ticker.C:
			closeNext = true
		case r, ok := <-w.incoming:
			if ok == false {
				return nil
			}
			if err := w.writeLine(r, nextName); err != nil {
				return err
			}

			if closeNext == false {
				continue
			}

			closeNext = false
			nextName, err = w.closeAndGetNextName()
			if err != nil {
				return err
			}
		}
	}
}
