package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/joho/godotenv"
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
		if !strings.HasSuffix(path, ".mp4") || info.IsDir() {
			return nil
		}

		if err != nil {
			return err
		}

		thumbPath := path + constants.ThumbFileSuffix

		if exists(thumbPath) {
			return nil
		}

		fmt.Println(path)

		cmd := exec.Command(context.Config.FfmpegPath,
			"-i", path,
			"-ss", "00:00:0",
			"-vframes", "1",
			thumbPath,
		)

		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			fmt.Println(err.Error())
			return nil
		}

		if err := cmd.Wait(); err != nil {
			fmt.Println(err.Error())
			return nil
		}

		return nil
	})

	if walkErr != nil {
		log.Fatalf("error when scanning: %v", walkErr)
	}
}
