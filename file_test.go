package gofat

import (
	"github.com/golang/mock/gomock"
	"os"
	"reflect"
	"testing"
)

func TestFile_Close(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "just close and reset all fields",
			fields: fields{
				fs:           &Fs{},
				path:         "any path",
				isDirectory:  true,
				isReadOnly:   true,
				isHidden:     true,
				isSystem:     true,
				firstCluster: 5,
				size:         6,
				stat:         entryHeaderFileInfo{},
				offset:       7,
			},
		},
	}

	fEmpty := File{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			if err := f.Close(); (err != nil) != tt.wantErr {
				t.Errorf("File.Close() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && *f != fEmpty {
				t.Errorf("File.Close() did not reset all fields: File = %v want = %v", *f, fEmpty)
			}
		})
	}
}

func TestFile_Read(t *testing.T) {
	type fields struct {
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		p []byte
	}
	type mock struct {
		readAtResult []byte
		readAtError  error
	}
	tests := []struct {
		name    string
		mock    mock
		fields  fields
		args    args
		wantN   int
		wantErr bool
	}{
		{
			name: "simple file",
			mock: mock{
				readAtResult: []byte{'H', 'e', 'l', 'l', '0', ' ', 'W', 'o', 'r', 'l', 'd'},
				readAtError:  nil,
			},
			fields: fields{
				size:         11,
				firstCluster: 0,
			},
			args: args{
				p: make([]byte, 11),
			},
			wantN:   11,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockFs := NewMockfatFileFs(mockCtrl)
			mockFs.EXPECT().
				readFileAt(tt.fields.firstCluster, tt.fields.offset, len(tt.args.p)).
				MaxTimes(1).
				Return(tt.mock.readAtResult, tt.mock.readAtError)

			f := &File{
				fs:           mockFs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}

			gotN, err := f.Read(tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("File.Read() = %v, want %v", gotN, tt.wantN)
			}

			mockCtrl.Finish()
		})
	}
}

func TestFile_ReadAt(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		p   []byte
		off int64
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantN   int
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			gotN, err := f.ReadAt(tt.args.p, tt.args.off)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.ReadAt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("File.ReadAt() = %v, want %v", gotN, tt.wantN)
			}
		})
	}
}

func TestFile_Seek(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		offset int64
		whence int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    int64
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			got, err := f.Seek(tt.args.offset, tt.args.whence)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Seek() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("File.Seek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Write(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		p []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantN   int
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			gotN, err := f.Write(tt.args.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("File.Write() = %v, want %v", gotN, tt.wantN)
			}
		})
	}
}

func TestFile_WriteAt(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		p   []byte
		off int64
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantN   int
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			gotN, err := f.WriteAt(tt.args.p, tt.args.off)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.WriteAt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("File.WriteAt() = %v, want %v", gotN, tt.wantN)
			}
		})
	}
}

func TestFile_Name(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			if got := f.Name(); got != tt.want {
				t.Errorf("File.Name() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Readdir(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		count int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []os.FileInfo
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			got, err := f.Readdir(tt.args.count)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Readdir() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("File.Readdir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Readdirnames(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		n int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			got, err := f.Readdirnames(tt.args.n)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Readdirnames() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("File.Readdirnames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Stat(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	tests := []struct {
		name    string
		fields  fields
		want    os.FileInfo
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			got, err := f.Stat()
			if (err != nil) != tt.wantErr {
				t.Errorf("File.Stat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("File.Stat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFile_Sync(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			if err := f.Sync(); (err != nil) != tt.wantErr {
				t.Errorf("File.Sync() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFile_Truncate(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		size int64
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			if err := f.Truncate(tt.args.size); (err != nil) != tt.wantErr {
				t.Errorf("File.Truncate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFile_WriteString(t *testing.T) {
	type fields struct {
		fs           fatFileFs
		path         string
		isDirectory  bool
		isReadOnly   bool
		isHidden     bool
		isSystem     bool
		firstCluster fatEntry
		size         uint32
		stat         os.FileInfo
		offset       int64
	}
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantRet int
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &File{
				fs:           tt.fields.fs,
				path:         tt.fields.path,
				isDirectory:  tt.fields.isDirectory,
				isReadOnly:   tt.fields.isReadOnly,
				isHidden:     tt.fields.isHidden,
				isSystem:     tt.fields.isSystem,
				firstCluster: tt.fields.firstCluster,
				size:         tt.fields.size,
				stat:         tt.fields.stat,
				offset:       tt.fields.offset,
			}
			gotRet, err := f.WriteString(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("File.WriteString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotRet != tt.wantRet {
				t.Errorf("File.WriteString() = %v, want %v", gotRet, tt.wantRet)
			}
		})
	}
}
