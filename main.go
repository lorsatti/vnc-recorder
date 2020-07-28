package main

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"
	vnc "github.com/amitbet/vnc2video"
	"github.com/amitbet/vnc2video/encoders"
)

func init() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.WarnLevel)
}

func main() {
	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Usage = "Connect to a vnc server and record the screen to a video."
	app.Version = "1.0"
	app.Authors = []cli.Author{
		{
			Name:  "Daniel Widerin",
			Email: "daniel@widerin.net",
		},
	}
	app.Action = recorder
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "ffmpeg",
			Value:  "ffmpeg",
			Usage:  "Which ffmpeg executable to use",
			EnvVar: "VR_FFMPEG_BIN",
		},
		cli.StringFlag{
			Name:   "host",
			Value:  "localhost",
			Usage:  "VNC host",
			EnvVar: "VR_VNC_HOST",
		},
		cli.IntFlag{
			Name:   "port",
			Value:  5900,
			Usage:  "VNC port",
			EnvVar: "VR_VNC_PORT",
		},
		cli.StringFlag{
			Name:   "password",
			Value:  "secret",
			Usage:  "Password to connect to the VNC host",
			EnvVar: "VR_VNC_PASSWORD",
		},
		cli.IntFlag{
			Name:   "framerate",
			Value:  30,
			Usage:  "Framerate to record",
			EnvVar: "VR_FRAMERATE",
		},
		cli.StringFlag{
			Name:   "outfile",
			Value:  "output.mp4",
			Usage:  "Output file to record to.",
			EnvVar: "VR_OUTFILE",
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func recorder(c *cli.Context) error {
	address := fmt.Sprintf("%s:%d", c.String("host"), c.Int("port"))
	nc, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		log.Fatalf("Error connecting to VNC host. %v", err)
		return err
	}
	defer nc.Close()

	log.Infof("Connected to %s", address)

	// Negotiate connection with the server.
	cchServer := make(chan vnc.ServerMessage)
	cchClient := make(chan vnc.ClientMessage)
	errorCh := make(chan error)

	ccfg := &vnc.ClientConfig{
		SecurityHandlers: []vnc.SecurityHandler{
			// &vnc.ClientAuthATEN{Username: []byte(os.Args[2]), Password: []byte(os.Args[3])}
			&vnc.ClientAuthVNC{Password: []byte(c.String("password"))},
			&vnc.ClientAuthNone{},
		},
		DrawCursor:      true,
		PixelFormat:     vnc.PixelFormat32bit,
		ClientMessageCh: cchClient,
		ServerMessageCh: cchServer,
		Messages:        vnc.DefaultServerMessages,
		Encodings: []vnc.Encoding{
			&vnc.RawEncoding{},
			&vnc.TightEncoding{},
			&vnc.HextileEncoding{},
			&vnc.ZRLEEncoding{},
			&vnc.CopyRectEncoding{},
			&vnc.CursorPseudoEncoding{},
			&vnc.CursorPosPseudoEncoding{},
			&vnc.ZLibEncoding{},
			&vnc.RREEncoding{},
		},
		ErrorCh: errorCh,
	}

	cc, err := vnc.Connect(context.Background(), nc, ccfg)
	defer cc.Close()

	screenImage := cc.Canvas
	if err != nil {
		log.Fatalf("Error negotiating connection to VNC host. %v", err)
		return err
	}

	ffmpegPath, err := exec.LookPath(c.String("ffmpeg"))
	if err != nil {
		panic(err)
	}
	log.Infof("Using %s for encoding", ffmpegPath)
	vcodec := &encoders.Encoder{
		BinPath:   ffmpegPath,
		Framerate: c.Int("framerate"),
		Cmd: exec.Command(ffmpegPath,
			"-f", "image2pipe",
			"-vcodec", "ppm",
			"-framerate", strconv.Itoa(c.Int("framerate")),
			"-i", "-",
			"-an", // no audio
			"-y",
			"-vcodec", "libx264", //"libvpx",//"libvpx-vp9"//"libx264"
			"-pix_fmt", "yuv420p",
			c.String("outfile"),
		),
	}

	go vcodec.Run()
	vcodecRunning := true

	cc.SetEncodings([]vnc.EncodingType{
		vnc.EncCursorPseudo,
		vnc.EncPointerPosPseudo,
		vnc.EncCopyRect,
		vnc.EncTight,
		vnc.EncZRLE,
		vnc.EncHextile,
		vnc.EncZlib,
		vnc.EncRRE,
	})

	go func() {
		for {
			timeStart := time.Now()

			if vcodecRunning {
				vcodec.Encode(screenImage.Image)
			} else {
				log.Info("stop encoding")
				return
			}

			timeTarget := timeStart.Add((1000 / time.Duration(vcodec.Framerate)) * time.Millisecond)
			timeLeft := timeTarget.Sub(time.Now())
			if timeLeft > 0 {
				time.Sleep(timeLeft)
			}
		}
	}()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	frameBufferReq := 0
	timeStart := time.Now()

	defer gracefulClose(vcodec, &vcodecRunning)

	for {
		select {
		case err := <-errorCh:
			panic(err)
		case msg := <-cchClient:
			log.Debugf("Received client message type:%v msg:%v\n", msg.Type(), msg)
		case msg := <-cchServer:
			if msg.Type() == vnc.FramebufferUpdateMsgType {
				secsPassed := time.Now().Sub(timeStart).Seconds()
				frameBufferReq++
				reqPerSec := float64(frameBufferReq) / secsPassed
				//counter++
				//jpeg.Encode(out, screenImage, nil)
				///vcodec.Encode(screenImage)
				log.Debugf("reqs=%d, seconds=%f, Req Per second= %f", frameBufferReq, secsPassed, reqPerSec)

				reqMsg := vnc.FramebufferUpdateRequest{Inc: 1, X: 0, Y: 0, Width: cc.Width(), Height: cc.Height()}
				//cc.ResetAllEncodings()
				reqMsg.Write(cc)
			}
		case signal := <-sigc:
			if signal != nil {
				log.Info(signal, " received, exit.")
				return nil
			}
		}
	}
}

func gracefulClose(e *encoders.Encoder, vcodecRunning *bool) {
	// stop sending images to encoder
	*vcodecRunning = false
	// give some time to stop encoding
	time.Sleep(1 * time.Second)
	// close pipe
	e.Close()
	// give some time to write the file
	time.Sleep(5 * time.Second)
}
