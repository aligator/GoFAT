package gofat

import (
	"github.com/spf13/afero"
	"io"
	"strings"
	"testing"
	"testing/fstest"
)

// TestGoFS tests the own, deprecated compatibility layer to io.FS.
func TestGoFS(t *testing.T) {
	gofs := GoFs{*testingNew(t, testFileReader(fat32))}
	if err := fstest.TestFS(gofs, "DoNotEdit_tests/HelloWorldThisIsALoongFileName.txt", "DoNotEdit_tests/README.md"); err != nil {
		t.Fatal(err)
	}
}

// TestIOFS tests the use with the afero.IOFS compatibility layer to io.FS.
func TestIOFS(t *testing.T) {
	iofs := afero.IOFS{testingNew(t, testFileReader(fat32))}
	if err := fstest.TestFS(iofs, "DoNotEdit_tests/HelloWorldThisIsALoongFileName.txt", "DoNotEdit_tests/README.md"); err != nil {
		t.Fatal(err)
	}
}

func TestNewGoFS(t *testing.T) {
	type args struct {
		reader io.ReadSeeker
	}
	tests := []struct {
		name       string
		args       args
		wantNotNil bool
		wantErr    bool
	}{
		{
			name: "FAT32 test image",
			args: args{
				reader: testFileReader(fat32),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "FAT16 test image",
			args: args{
				reader: testFileReader(fat16),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "no FAT file",
			args: args{
				reader: strings.NewReader("This is no FAT file"),
			},
			wantNotNil: false,
			wantErr:    true,
		},
		{
			name: "fat32 invalid sectors per cluster test image",
			args: args{
				reader: testFileReader(fat32InvalidSectorsPerCluster),
			},
			wantNotNil: false,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGoFS(tt.args.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got != nil) != tt.wantNotNil {
				t.Errorf("New() = %v, wantNotNil %v", got, tt.wantNotNil)
			}
		})
	}
}

func TestNewIOFS(t *testing.T) {
	type args struct {
		reader io.ReadSeeker
	}
	tests := []struct {
		name         string
		args         args
		wantNotEmpty bool
		wantErr      bool
	}{
		{
			name: "FAT32 test image",
			args: args{
				reader: testFileReader(fat32),
			},
			wantNotEmpty: true,
			wantErr:      false,
		},
		{
			name: "FAT16 test image",
			args: args{
				reader: testFileReader(fat16),
			},
			wantNotEmpty: true,
			wantErr:      false,
		},
		{
			name: "no FAT file",
			args: args{
				reader: strings.NewReader("This is no FAT file"),
			},
			wantNotEmpty: false,
			wantErr:      true,
		},
		{
			name: "fat32 invalid sectors per cluster test image",
			args: args{
				reader: testFileReader(fat32InvalidSectorsPerCluster),
			},
			wantNotEmpty: false,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewIOFS(tt.args.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got != (afero.IOFS{})) != tt.wantNotEmpty {
				t.Errorf("New() = %v, wantNotEmpty %v", got, tt.wantNotEmpty)
			}
		})
	}
}

func TestNewGoFSSkipChecks(t *testing.T) {
	type args struct {
		reader io.ReadSeeker
	}
	tests := []struct {
		name       string
		args       args
		wantNotNil bool
		wantErr    bool
	}{
		{
			name: "FAT32 test image",
			args: args{
				reader: testFileReader(fat32),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "FAT16 test image",
			args: args{
				reader: testFileReader(fat16),
			},
			wantNotNil: true,
			wantErr:    false,
		},
		{
			name: "no FAT file",
			args: args{
				reader: strings.NewReader("This is no FAT file"),
			},
			wantNotNil: false,
			wantErr:    true,
		},
		{
			name: "fat32 invalid sectors per cluster test image",
			args: args{
				reader: testFileReader(fat32InvalidSectorsPerCluster),
			},
			wantNotNil: true,
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewGoFSSkipChecks(tt.args.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSkipChecks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got != nil) != tt.wantNotNil {
				t.Errorf("New() = %v, wantNotNil %v", got, tt.wantNotNil)
			}
		})
	}
}

func TestNewIOFSSkipChecks(t *testing.T) {
	type args struct {
		reader io.ReadSeeker
	}
	tests := []struct {
		name         string
		args         args
		wantNotEmpty bool
		wantErr      bool
	}{
		{
			name: "FAT32 test image",
			args: args{
				reader: testFileReader(fat32),
			},
			wantNotEmpty: true,
			wantErr:      false,
		},
		{
			name: "FAT16 test image",
			args: args{
				reader: testFileReader(fat16),
			},
			wantNotEmpty: true,
			wantErr:      false,
		},
		{
			name: "no FAT file",
			args: args{
				reader: strings.NewReader("This is no FAT file"),
			},
			wantNotEmpty: false,
			wantErr:      true,
		},
		{
			name: "fat32 invalid sectors per cluster test image",
			args: args{
				reader: testFileReader(fat32InvalidSectorsPerCluster),
			},
			wantNotEmpty: true,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewIOFSSkipChecks(tt.args.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSkipChecks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got != (afero.IOFS{})) != tt.wantNotEmpty {
				t.Errorf("New() = %v, wantNotEmpty %v", got, tt.wantNotEmpty)
			}
		})
	}
}
