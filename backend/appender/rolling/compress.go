package rolling

import (
	"compress/gzip"
	"io"
)

type Compression interface {
	Compress(in io.Reader, out io.Writer) error
	Extension() string
}

type CompressGZip struct {
	Level int
}

func (c *CompressGZip) Extension() string {
	return "gz"
}

func (c *CompressGZip) Compress(in io.Reader, out io.Writer) (err error) {
	w, errOpen := gzip.NewWriterLevel(out, c.Level)
	if errOpen != nil {
		return errOpen
	}

	defer func() {
		cerr := w.Close()
		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(w, in); err == nil {
		err = w.Flush()
	}
	return err
}
