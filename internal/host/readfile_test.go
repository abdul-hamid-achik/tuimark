package host

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadRegularReadsFilesAndSymlinksToThem(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.json")
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRegular(file, 0)
	if err != nil || string(got) != "{}" {
		t.Fatalf("ReadRegular = %q, %v", got, err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(file, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if got, err := ReadRegular(link, 0); err != nil || string(got) != "{}" {
		t.Errorf("through a symlink: %q, %v", got, err)
	}
}

func TestReadRegularRefusesWhatIsNotAFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadRegular(dir, 0); !errors.Is(err, ErrNotRegular) {
		t.Errorf("a directory: %v, want ErrNotRegular", err)
	}
	if _, err := ReadRegular(filepath.Join(dir, "absent"), 0); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: %v, want ErrNotExist", err)
	}
}

func TestReadRegularRefusesAFileOverTheLimit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "big")
	if err := os.WriteFile(file, make([]byte, 11), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegular(file, 10); !errors.Is(err, ErrTooLarge) {
		t.Errorf("11 bytes under a limit of 10: %v, want ErrTooLarge", err)
	}
	if got, err := ReadRegular(file, 11); err != nil || len(got) != 11 {
		t.Errorf("11 bytes under a limit of 11: %d, %v", len(got), err)
	}
}

// The default limit is MaxReadBytes: a sparse file one byte over it is
// refused, one of exactly that size is not.
func TestReadRegularDefaultLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("reads 64 MiB")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "big")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(MaxReadBytes + 1); err != nil {
		t.Skipf("no sparse files: %v", err)
	}
	if _, err := ReadRegular(file, 0); !errors.Is(err, ErrTooLarge) {
		t.Errorf("MaxReadBytes+1 bytes: %v, want ErrTooLarge", err)
	}
	if err := f.Truncate(MaxReadBytes); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadRegular(file, 0); err != nil || len(got) != MaxReadBytes {
		t.Errorf("MaxReadBytes bytes: %d, %v", len(got), err)
	}
}
