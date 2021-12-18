package application_context

import (
	"bytes"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"io"
	"os"
	"testing"
)

func getMeTheFileOrPanic(path string) io.Reader {
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
		file         io.Reader
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &MahresourcesContext{
				fs:             tt.fields.fs,
				db:             tt.fields.db,
				altFileSystems: tt.fields.altFileSystems,
			}
			if err := ctx.createThumbFromVideo(tt.args.file, tt.args.resultBuffer); (err != nil) != tt.wantErr {
				t.Errorf("createThumbFromVideo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
