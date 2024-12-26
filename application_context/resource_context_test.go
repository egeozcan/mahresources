package application_context

import (
	"bytes"
	"github.com/joho/godotenv"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

var context *MahresourcesContext

func init() {
	curPath, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
	}

	log.Println(curPath)

	filesToTry := []string{".test.env", ".env"}
	curPathHasEnvFile := func(curPath string) *string {
		for _, file := range filesToTry {
			if _, err := os.Stat(filepath.Join(curPath, file)); err == nil {
				return &file
			}
		}
		return nil
	}

	for {
		log.Println("trying", curPath)

		if len(curPath) <= 3 {
			log.Fatal("no env file found!")
		}

		file := curPathHasEnvFile(curPath)

		if file == nil {
			log.Println("going up", curPath)
			curPath = filepath.Dir(curPath)
			log.Println("new path", curPath)
			log.Println(curPath)
			continue
		}

		_ = godotenv.Load(filepath.Join(curPath, *file))
		break
	}

	context, _, _ = CreateContext()
}

func getMeTheFileOrPanic(path string) io.ReadSeeker {
	file, err := os.Open(path)

	if err != nil {
		// no file... panic!!!
		panic(err)
	}

	// got the file!!! DO NOT PANIC.
	return file
}

func TestMahresourcesContext_createThumbFromVideo(t *testing.T) {
	if err := context.createThumbFromVideo(getMeTheFileOrPanic("../test_data/pexels-thirdman-5862328.mp4"), bytes.NewBuffer(make([]byte, 0))); err != nil {
		t.Errorf("createThumbFromVideo() error = %v", err)
		return
	}
}
