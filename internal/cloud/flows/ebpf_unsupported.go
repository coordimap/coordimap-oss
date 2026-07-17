//go:build !linux

package flows

import "errors"

var errEBPFUnsupported = errors.New("eBPF flow capture is supported only on Linux")

type Probe struct {
	Samples <-chan []byte
}

func AttachProbe(string) (*Probe, error) {
	return nil, errEBPFUnsupported
}

func (*Probe) Detach() {}
