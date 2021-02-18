package gofat

import (
	"testing"
	"testing/fstest"
)

func TestGoFS(t *testing.T) {
	gofs := GoFs{*testingNew(t, testFileReader(fat32))}
	if err := fstest.TestFS(gofs, "DoNotEdit_tests/HelloWorldThisIsALoongFileName.txt", "DoNotEdit_tests/README.md"); err != nil {
		t.Fatal(err)
	}
}
