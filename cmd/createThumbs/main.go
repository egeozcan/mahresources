package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"io/fs"
	"log"
	"mahresources/application_context"
	"mahresources/constants"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func main() {
	_ = godotenv.Load(".env")

	target := flag.String("target", "", "Target directory to create thumbnails")

	flag.Parse()

	if *target == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	context, _, _ := application_context.CreateContext()

	fmt.Println("target", *target)

	stat, err := os.Stat(*target)

	if err != nil {
		log.Fatalf("error when opening the target: %v", err)
	}

	if !stat.IsDir() {
		log.Fatalf("target is not a dir")
	}

	walkErr := filepath.Walk(*target, func(path string, info fs.FileInfo, err error) error {
		fmt.Println(path)

		if !strings.HasSuffix(path, ".mp4") || info.IsDir() {
			return nil
		}

		resultBuffer := bytes.NewBuffer(make([]byte, 0))

		if err != nil {
			return err
		}

		thumbPath := path + constants.ThumbFileSuffix

		if exists(thumbPath) {
			return nil
		}

		cmd := exec.Command(context.Config.FfmpegPath,
			"-i", path,
			"-ss", "00:00:0",
			"-vframes", "1",
			"-c:v", "png",
			"-f", "image2pipe",
			"-",
		)

		cmd.Stderr = os.Stderr
		cmd.Stdout = resultBuffer

		if err := cmd.Start(); err != nil {
			return err
		}

		if err := cmd.Wait(); err != nil {
			return err
		}

		thumbFile, err := os.Create(thumbPath)

		if err != nil {
			return err
		}

		_, err = io.Copy(thumbFile, resultBuffer)

		if err != nil {
			return err
		}

		return nil
	})

	if walkErr != nil {
		log.Fatalf("error when scanning: %v", walkErr)
	}
}
