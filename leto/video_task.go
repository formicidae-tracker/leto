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

var ffmpegCommandName = "ffmpeg"

type FFMpegCommand struct {
	log    *os.File
	ecmd   *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func NewFFMpegCommand(args []string, logFileName string) (*FFMpegCommand, error) {
	cmd := &FFMpegCommand{
		ecmd: exec.Command(ffmpegCommandName, args...),
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

func (cmd *FFMpegCommand) Stop() error {
	return cmd.ecmd.Process.Signal(os.Interrupt)
}

func (cmd *FFMpegCommand) Wait() error {
	return cmd.ecmd.Wait()
}

type VideoTask interface {
	Run(io.ReadCloser) error
}

type videoFilename struct {
	movie         string
	frameMatching string
	encodeLog     string
	saveLog       string
	streamLog     string
}

func NewBaseVideoName(basedir string) videoFilename {
	return videoFilename{
		movie:         filepath.Join(basedir, "stream.mp4"),
		frameMatching: filepath.Join(basedir, "stream.frame-matching.txt"),
		encodeLog:     filepath.Join(basedir, "encoding.log"),
		saveLog:       filepath.Join(basedir, "save.log"),
		streamLog:     filepath.Join(basedir, "streaming.log"),
	}

}

func (fn videoFilename) InstantiateWithoutOverwrite() (videoFilename, error) {
	res := videoFilename{}
	var err error

	res.movie, _, err = FilenameWithoutOverwrite(fn.movie)
	if err != nil {
		return res, err
	}

	res.frameMatching, _, err = FilenameWithoutOverwrite(fn.frameMatching)
	if err != nil {
		return res, err
	}

	res.encodeLog, _, err = FilenameWithoutOverwrite(fn.encodeLog)
	if err != nil {
		return res, err
	}

	res.saveLog, _, err = FilenameWithoutOverwrite(fn.saveLog)
	if err != nil {
		return res, err
	}

	res.streamLog, _, err = FilenameWithoutOverwrite(fn.streamLog)
	if err != nil {
		return res, err
	}
	return res, nil
}

type videoTaskConfig struct {
	baseFileName videoFilename

	hostname string

	period time.Duration

	fps         float64
	bitrate     int
	maxBitrate  int
	destAddress string
	resolution  string
	quality     string
	tune        string
}

func newVideoTaskConfig(basedir string, fps float64, config leto.StreamConfiguration) (videoTaskConfig, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return videoTaskConfig{}, err
	}

	return videoTaskConfig{
		hostname:     hostname,
		baseFileName: NewBaseVideoName(basedir),
		fps:          fps,
		bitrate:      *config.BitRateKB,
		maxBitrate:   int(float64(*config.BitRateKB) * *config.BitRateMaxRatio),
		destAddress:  *config.Host,
		resolution:   "",
		quality:      *config.Quality,
		tune:         *config.Tune,

		period: 2 * time.Hour,
	}, nil
}

type videoTask struct {
	wg sync.WaitGroup

	config videoTaskConfig

	encodeCmd, streamCmd, saveCmd *FFMpegCommand

	frameCorrespondance *os.File

	logger *log.Logger
}

func NewVideoManager(basedir string, fps float64, config leto.StreamConfiguration) (VideoTask, error) {
	conf, err := newVideoTaskConfig(basedir, fps, config)
	if err != nil {
		return nil, err
	}
	res := &videoTask{
		config: conf,
		logger: NewLogger("stream"),
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

func (m *videoTaskConfig) Check() error {
	if ok := presets[m.quality]; ok == false {
		return fmt.Errorf("unknown quality '%s'", m.quality)
	}
	if ok := tunes[m.tune]; ok == false {
		return fmt.Errorf("unknown tune '%s'", m.tune)
	}
	return nil
}

func (m *videoTask) Check() error {
	return m.config.Check()
}

func TeeCopy(dst, dstErrorIgnored io.Writer, src io.Reader) (int64, error) {
	size := 32 * 1024
	if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
		if l.N < 1 {
			size = 1
		} else {
			size = int(l.N)
		}
	}
	var n int64 = 0
	buf := make([]byte, size)
	for {
		nr, err := src.Read(buf)
		if err != nil {
			if err != io.EOF {
				return n, err
			}
			return n, nil
		}

		if nr <= 0 {
			continue
		}

		nw, errw1 := dst.Write(buf[0:nr])
		if errw1 != nil {
			return n + int64(nw), errw1
		}

		dstErrorIgnored.Write(buf[0:nr])
		n += int64(nr)

	}
}

func (s *videoTask) copyToSave() (int64, error) {
	defer s.saveCmd.Stdin().Close()
	return io.Copy(s.saveCmd.Stdin(), s.encodeCmd.Stdout())
}

func (s *videoTask) copyToSaveAndEncode() (int64, error) {
	defer s.saveCmd.Stdin().Close()
	defer s.streamCmd.Stdin().Close()
	return TeeCopy(s.saveCmd.Stdin(), s.streamCmd.Stdin(), s.encodeCmd.Stdout())
}

func (s *videoTask) copyRoutine() (int64, error) {
	if s.streamCmd != nil {
		return s.copyToSaveAndEncode()
	}
	return s.copyToSave()
}

func (s *videoTask) startCommand(cmd *FFMpegCommand, commandName string) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := cmd.Wait(); err != nil {
			s.logger.Printf("%s ffmpeg command failed: %s", commandName, err)
		}
	}()
	return nil
}

func (s *videoTask) startTasks() error {
	filenames, err := s.config.baseFileName.InstantiateWithoutOverwrite()
	if err != nil {
		return err
	}
	s.frameCorrespondance, err = os.Create(filenames.frameMatching)
	if err != nil {
		return err
	}

	s.encodeCmd, err = NewFFMpegCommand(s.config.encodeCommandArgs(), filenames.encodeLog)
	if err != nil {
		return err
	}

	s.saveCmd, err = NewFFMpegCommand(s.config.saveCommandArgs(filenames.movie), filenames.saveLog)
	if err != nil {
		return err
	}

	streamArgs := s.config.streamCommandArgs()

	if len(streamArgs) > 0 {
		s.streamCmd, err = NewFFMpegCommand(streamArgs, filenames.streamLog)
		if err != nil {
			return err
		}
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		n, err := s.copyRoutine()
		if err != nil {
			s.logger.Printf("could not tranfer data between tasks: %s", err)
		}
		s.logger.Printf("written %s", ByteSize(n))
	}()

	if len(s.config.destAddress) > 0 {
		s.logger.Printf("starting streaming to %s", s.config.destAddress)
	}
	s.logger.Printf("starting saving to %s", filenames.movie)

	if err := s.startCommand(s.encodeCmd, "encode"); err != nil {
		return err
	}

	if err := s.startCommand(s.saveCmd, "save"); err != nil {
		return err
	}

	if s.streamCmd == nil {
		return nil
	}

	return s.startCommand(s.streamCmd, "stream")
}

func (s *videoTask) stopTasks() {
	if s.encodeCmd == nil {
		return
	}
	s.logger.Printf("stopping video tasks")
	s.encodeCmd.Stdin().Close()
	//s.logger.Printf("encode Stop(): %s",s.encodeCmd.Stop())
}

func (s *videoTask) waitTasks() {
	s.wg.Wait()
	s.encodeCmd = nil
	s.saveCmd = nil
	s.encodeCmd = nil
	if s.frameCorrespondance != nil {
		s.frameCorrespondance.Close()
	}
	s.frameCorrespondance = nil
}

func (s *videoTask) Run(muxed io.ReadCloser) (retError error) {
	defer func() {
		if retError != nil {
			s.logger.Printf("failed with error: %s", retError)
			muxed.Close()
		}
		s.stopTasks()
		s.waitTasks()
	}()

	header := make([]byte, 3*8)

	currentFrame := 0
	nextFile := time.Now().Add(s.config.period)

	headerError := 0
	maxHeaderTrials := 1920 * 1024 * 3 * 30
	frameWriteError := 0
	maxFrameRetries := 3
	for {
		_, err := io.ReadFull(muxed, header)
		if err != nil {
			if err == io.EOF || err == io.ErrClosedPipe {
				return nil
			}

			if headerError == 0 {
				s.logger.Printf("cannot read header: %s", err)
			}
			headerError += 1
			if headerError >= maxHeaderTrials {
				return fmt.Errorf("Could not read the header for more than %d times", maxHeaderTrials)
			}
			continue
		}

		if headerError != 0 {
			s.logger.Printf("header read error repeated %d time(s)", headerError)
			headerError = 0
		}

		actual := binary.LittleEndian.Uint64(header)
		width := binary.LittleEndian.Uint64(header[8:])
		height := binary.LittleEndian.Uint64(header[16:])

		if len(s.config.resolution) == 0 {
			s.config.resolution = fmt.Sprintf("%dx%d", width, height)
		}

		if s.encodeCmd == nil && s.streamCmd == nil && s.frameCorrespondance == nil {
			if err := s.startTasks(); err != nil {
				return fmt.Errorf("could not start stream tasks: %w", err)
			}
			currentFrame = 0
			nextFile = time.Now().Add(s.config.period)
		}

		fmt.Fprintf(s.frameCorrespondance, "%d %d\n", currentFrame, actual)
		_, err = io.CopyN(s.encodeCmd.Stdin(), muxed, int64(3*width*height))
		if err != nil {
			s.logger.Printf("cannot copy frame: %v", err)
			frameWriteError += 1
			if frameWriteError >= maxFrameRetries {
				return fmt.Errorf("stop after encode in error: %w", err)
			}
			if err != os.ErrClosed {
				s.stopTasks()
				s.waitTasks()
			}
		}
		currentFrame += 1

		now := time.Now()
		if now.After(nextFile) == true {
			s.logger.Printf("creating new film segment after %s", s.config.period)

			s.stopTasks()

			s.waitTasks()
			s.logger.Printf("written %d frames", currentFrame)

		}
	}

}

func (s *videoTaskConfig) encodeCommandArgs() []string {
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

func (s *videoTaskConfig) streamCommandArgs() []string {
	if len(s.destAddress) == 0 {
		return []string{}
	}
	return []string{"-hide_banner",
		"-loglevel", "warning",
		"-f", "flv",
		"-i", "-",
		"-vcodec", "copy",
		fmt.Sprintf("rtmp://%s/olympus/%s.flv", s.destAddress, s.hostname),
	}
}

func (s *videoTaskConfig) saveCommandArgs(file string) []string {
	return []string{"-hide_banner",
		"-loglevel", "warning",
		"-f", "flv",
		"-i", "-",
		"-vcodec", "copy",
		file}
}
