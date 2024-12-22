package application_context

import (
	"bytes"
	"github.com/joho/godotenv"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"io"
	"log"
	"mahresources/lib"
	"os"
	"path"
	"testing"
)

var context *MahresourcesContext

func init() {
	curPath, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
	}

	filesToTry := []string{".test.env", ".env"}
	curPathHasEnvFile := func(curPath string) *string {
		for _, file := range filesToTry {
			if _, err := os.Stat(path.Join(curPath, file)); err == nil {
				return &file
			}
		}
		return nil
	}

	for true {
		if len(curPath) <= 3 {
			log.Fatal("no env file found")
		}

		file := curPathHasEnvFile(curPath)

		if file == nil {
			curPath = path.Join(curPath, "..")
			continue
		}

		_ = godotenv.Load(path.Join(curPath, *file))
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
	type fields struct {
		fs             afero.Fs
		db             *gorm.DB
		dbType         string
		altFileSystems map[string]afero.Fs
	}
	type args struct {
		file         io.ReadSeeker
		resultBuffer *bytes.Buffer
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "",
			fields: fields{
				fs:             nil,
				db:             nil,
				dbType:         "",
				altFileSystems: nil,
			},
			args: args{
				file:         getMeTheFileOrPanic("../test_data/pexels-thirdman-5862328.mp4"),
				resultBuffer: bytes.NewBuffer(make([]byte, 0)),
			},
			wantErr: false,
		},
	}

	lock := lib.NewIDLock[uint](uint(1))
	videoThumbGenLock := lib.NewIDLock[uint](uint(1))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &MahresourcesContext{
				fs:             tt.fields.fs,
				db:             tt.fields.db,
				altFileSystems: tt.fields.altFileSystems,
				locks: MahresourcesLocks{
					ThumbnailGenerationLock:      lock,
					VideoThumbnailGenerationLock: videoThumbGenLock,
				},
			}
			if err := ctx.createThumbFromVideo(tt.args.file, tt.args.resultBuffer, 1); (err != nil) != tt.wantErr {
				t.Errorf("createThumbFromVideo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestMahresourcesContext_GetSimilarResources(t *testing.T) {
	type args struct {
		id uint
	}
	tests := []struct {
		name    string
		context *MahresourcesContext
		args    args
		wantLen int
		wantErr bool
	}{
		{name: "Gets us something", context: context, args: args{id: 1}, wantLen: 1, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.context
			got, err := ctx.GetSimilarResources(tt.args.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSimilarResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(*got) != tt.wantLen {
				t.Errorf("GetSimilarResources() got = %v, want %v", len(*got), tt.wantLen)
			}
		})
	}
}
