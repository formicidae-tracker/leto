package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/formicidae-tracker/leto"
)

type FFMpegCommand struct {
	log    *os.File
	ecmd   *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func NewFFMpegCommand(args []string, streamType string, logFileName string) (*FFMpegCommand, error) {
	cmd := &FFMpegCommand{
		ecmd: exec.Command("ffmpeg", args...),
	}
	var err error
	// Close on exec will be set by go runtime, ensuring this file
	// will be closed on our side, and simply inherited by the ffmpeg
	// child process (and the OS always close its stderr ;))
	cmd.log, err = os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	cmd.ecmd.Stderr = cmd.log
	cmd.stdin, err = cmd.ecmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	cmd.stdout, err = cmd.ecmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

func (cmd *FFMpegCommand) Stdin() io.WriteCloser {
	return cmd.stdin
}

func (cmd *FFMpegCommand) Stdout() io.ReadCloser {
	return cmd.stdout
}

func (cmd *FFMpegCommand) Start() error {
	return cmd.ecmd.Start()
}

func (cmd *FFMpegCommand) Stop() {
	if cmd.ecmd.Process == nil {
		return
	}
	cmd.ecmd.Process.Signal(os.Interrupt)
}

func (cmd *FFMpegCommand) Wait() error {
	//todo close log file ???

	return cmd.ecmd.Wait()
}

type VideoManager struct {
	mx sync.Mutex
	wg sync.WaitGroup

	period time.Duration

	baseMovieName     string
	baseFrameMatching string
	encodeLogBase     string
	streamLogBase     string
	saveLogBase       string

	encodeCmd, streamCmd, saveCmd *FFMpegCommand

	frameCorrespondance *os.File

	host string

	fps         float64
	bitrate     int
	maxBitrate  int
	destAddress string
	resolution  string
	quality     string
	tune        string

	logger *log.Logger
}

func NewVideoManager(basedir string, fps float64, config leto.StreamConfiguration) (*VideoManager, error) {
	res := &VideoManager{
		baseMovieName:     filepath.Join(basedir, "stream.mp4"),
		baseFrameMatching: filepath.Join(basedir, "stream.frame-matching.txt"),
		encodeLogBase:     filepath.Join(basedir, "encoding.log"),
		streamLogBase:     filepath.Join(basedir, "streaming.log"),
		saveLogBase:       filepath.Join(basedir, "save.log"),
		fps:               fps,
		bitrate:           *config.BitRateKB,
		maxBitrate:        int(float64(*config.BitRateKB) * *config.BitRateMaxRatio),
		destAddress:       *config.Host,
		resolution:        "",
		quality:           *config.Quality,
		tune:              *config.Tune,
		period:            2 * time.Hour,
		logger:            NewLogger("stream"),
	}
	if err := res.Check(); err != nil {
		return nil, err
	}
	return res, nil
}

var presets = map[string]bool{
	"ultrafast": true,
	"superfast": true,
	"veryfast":  true,
	"faster":    true,
	"fast":      true,
	"medium":    true,
	"slow":      true,
	"slower":    true,
	"veryslow":  true,
}

var tunes = map[string]bool{
	"film":        true,
	"animation":   true,
	"grain":       true,
	"stillimage":  true,
	"fastdecode":  true,
	"zerolatency": true,
}

func (m *VideoManager) Check() error {
	if ok := presets[m.quality]; ok == false {
		return fmt.Errorf("unknown quality '%s'", m.quality)
	}
	if ok := tunes[m.tune]; ok == false {
		return fmt.Errorf("unknown tune '%s'", m.tune)
	}
	return nil
}

func (s *VideoManager) waitUnsafe() {

	if s.encodeCmd != nil {
		s.logger.Printf("waiting for encoding to stop")
		s.encodeCmd.Wait()
	}

	if s.saveCmd != nil {
		s.saveCmd.Stop()
		s.logger.Printf("waiting for saving to stop")
		s.saveCmd.Wait()
		s.saveCmd.Stdout().Close()
	}

	if s.streamCmd != nil {
		s.streamCmd.Stop()
		s.logger.Printf("waiting for streaming to stop")
		s.streamCmd.Wait()
		s.streamCmd.Stdout().Close()
	}

	s.encodeCmd = nil
	s.saveCmd = nil
	s.streamCmd = nil

	if s.frameCorrespondance != nil {
		s.frameCorrespondance.Close()
		s.frameCorrespondance = nil
	}

}

func (s *VideoManager) Wait() {
	s.wg.Wait()

	s.mx.Lock()
	s.waitUnsafe()
	s.mx.Unlock()
}

func TeeCopy(dst, dstErrorIgnored io.Writer, src io.Reader) error {
	size := 32 * 1024
	if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
		if l.N < 1 {
			size = 1
		} else {
			size = int(l.N)
		}
	}
	buf := make([]byte, size)
	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			//try t
			nw, errw1 := dst.Write(buf[0:nr])
			if errw1 != nil {
				return errw1
			}
			if nw != nr {
				return io.ErrShortWrite
			}

			dstErrorIgnored.Write(buf[0:nr])
		}

		if err != nil {
			if err != io.EOF {
				return err
			}
			return nil
		}
	}
}

func (s *VideoManager) startTasks() error {
	encodeLogName, _, err := FilenameWithoutOverwrite(s.encodeLogBase)
	if err != nil {
		return err
	}

	streamLogName, _, err := FilenameWithoutOverwrite(s.streamLogBase)
	if err != nil {
		return err
	}

	saveLogName, _, err := FilenameWithoutOverwrite(s.saveLogBase)
	if err != nil {
		return err
	}

	mName, _, err := FilenameWithoutOverwrite(s.baseMovieName)
	if err != nil {
		return err
	}

	cfName, _, err := FilenameWithoutOverwrite(s.baseFrameMatching)
	if err != nil {
		return err
	}
	s.frameCorrespondance, err = os.Create(cfName)
	if err != nil {
		return err
	}

	s.encodeCmd, err = NewFFMpegCommand(s.encodeCommandArgs(), "encode", encodeLogName)
	if err != nil {
		return err
	}

	s.saveCmd, err = NewFFMpegCommand(s.saveCommandArgs(mName), "save", saveLogName)
	if err != nil {
		return err
	}
	streamArgs := s.streamCommandArgs()

	copyRoutine := func() error {
		_, err := io.Copy(s.saveCmd.Stdin(), s.encodeCmd.Stdout())
		s.encodeCmd.Stdout().Close()
		s.saveCmd.Stdin().Close()
		return err
	}

	if len(streamArgs) > 0 {
		s.streamCmd, err = NewFFMpegCommand(streamArgs, "stream", streamLogName)
		if err != nil {
			return err
		}
		copyRoutine = func() error {
			err := TeeCopy(s.saveCmd.Stdin(), s.streamCmd.Stdin(), s.encodeCmd.Stdout())
			s.encodeCmd.Stdout().Close()
			s.saveCmd.Stdin().Close()
			s.streamCmd.Stdin().Close()
			return err
		}
	}

	s.wg.Add(1)
	go func() {
		err := copyRoutine()
		if err != nil {
			s.logger.Printf("Could not tranfer data between tasks: %s", err)
		}
		s.logger.Printf("Copying routine finished")
		s.wg.Done()
	}()

	s.logger.Printf("Starting streaming to %s and %s", mName, s.destAddress)
	err = s.encodeCmd.Start()
	if err != nil {
		return err
	}

	err = s.saveCmd.Start()
	if err != nil {
		return err
	}

	if s.streamCmd != nil {
		return s.streamCmd.Start()
	}

	return nil
}

func (s *VideoManager) stopTasks() {
	if s.encodeCmd == nil {
		return
	}

	s.logger.Printf("Stopping streaming tasks")
	s.encodeCmd.Stdin().Close()
	s.encodeCmd.Stop()
}

func (s *VideoManager) EncodeAndStreamMuxedStream(muxed io.Reader) {
	s.wg.Add(1)
	defer s.wg.Done()
	header := make([]byte, 3*8)
	var err error
	s.host, err = os.Hostname()
	if err != nil {
		s.logger.Printf("cannot get hostname: %s", err)
		return
	}

	currentFrame := 0
	nextFile := time.Now().Add(s.period)

	loggedSameError := 0
	maxHeaderTrials := 1920 * 1024 * 3 * 30
	for {
		_, err := io.ReadFull(muxed, header)
		if err != nil {
			if loggedSameError == 0 {
				s.logger.Printf("cannot read header: %s", err)
			}
			loggedSameError += 1
			if loggedSameError >= maxHeaderTrials {
				s.logger.Printf("Cannot read the header for more than %d times, quiting", maxHeaderTrials)
				return
			}
			if err == io.EOF || err == io.ErrClosedPipe {
				s.mx.Lock()
				defer s.mx.Unlock()
				s.stopTasks()
				return
			}
			continue
		}
		if loggedSameError != 0 {
			log.Printf("header read error repeated %d time(s)", loggedSameError)
			loggedSameError = 0
		}

		actual := binary.LittleEndian.Uint64(header)
		width := binary.LittleEndian.Uint64(header[8:])
		height := binary.LittleEndian.Uint64(header[16:])
		if len(s.resolution) == 0 {
			s.resolution = fmt.Sprintf("%dx%d", width, height)
		}
		s.mx.Lock()
		if s.encodeCmd == nil && s.streamCmd == nil && s.frameCorrespondance == nil {
			if err := s.startTasks(); err != nil {
				s.mx.Unlock()
				s.logger.Printf("Could not start stream tasks: %s", err)
				return
			}
			currentFrame = 0
		}
		s.mx.Unlock()

		fmt.Fprintf(s.frameCorrespondance, "%d %d\n", currentFrame, actual)
		_, err = io.CopyN(s.encodeCmd.Stdin(), muxed, int64(3*width*height))
		if err != nil {
			s.logger.Printf("cannot copy frame: %s", err)
		}
		currentFrame += 1

		now := time.Now()
		if now.After(nextFile) == true {
			log.Printf("Creating new film segment after %s", s.period)
			s.mx.Lock()
			s.stopTasks()
			s.waitUnsafe()
			s.mx.Unlock()
			nextFile = now.Add(s.period)
		}
	}

}

func (s *VideoManager) encodeCommandArgs() []string {
	vbr := fmt.Sprintf("%dk", s.bitrate)
	maxbr := fmt.Sprintf("%dk", s.maxBitrate)
	return []string{"-hide_banner",
		"-loglevel", "warning",
		"-f", "rawvideo",
		"-vcodec", "rawvideo",
		"-pixel_format", "rgb24",
		"-video_size", s.resolution,
		"-framerate", fmt.Sprintf("%f", s.fps),
		"-i", "-",
		"-c:v:0", "libx264",
		"-g", fmt.Sprintf("%d", int(2*s.fps)),
		"-keyint_min", fmt.Sprintf("%d", int(s.fps)),
		"-b:v", vbr,
		"-maxrate", maxbr,
		"-bufsize", vbr,
		"-pix_fmt",
		"yuv420p",
		"-s", s.resolution,
		"-preset", s.quality,
		"-tune", s.tune,
		"-f", "flv",
		"-"}
}

func (s *VideoManager) streamCommandArgs() []string {
	if len(s.destAddress) == 0 {
		return []string{}
	}
	return []string{"-hide_banner",
		"-loglevel", "warning",
		"-f", "flv",
		"-i", "-",
		"-vcodec", "copy",
		fmt.Sprintf("rtmp://%s/olympus/%s.flv", s.destAddress, s.host),
	}
}

func (s *VideoManager) saveCommandArgs(file string) []string {
	return []string{"-hide_banner",
		"-loglevel", "warning",
		"-f", "flv",
		"-i", "-",
		"-vcodec", "copy",
		file}
}
