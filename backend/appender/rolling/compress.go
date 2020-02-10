// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0

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
