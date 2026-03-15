package lsp

import "io"

type readWriteCloser struct {
	io.ReadCloser
	io.WriteCloser
}

func newReadWriteCloser(r io.ReadCloser, w io.WriteCloser) io.ReadWriteCloser {
	return &readWriteCloser{r, w}
}

func (rwc *readWriteCloser) Close() error {
	rerr := rwc.ReadCloser.Close()
	werr := rwc.WriteCloser.Close()
	if rerr != nil {
		return rerr
	}
	return werr
}
