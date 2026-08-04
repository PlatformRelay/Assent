package render

import "errors"

// ErrFindingThreadNotImplemented is returned by the S01 stub until E8-S08.
var ErrFindingThreadNotImplemented = errors.New("render: finding-thread layout not implemented (E8-S08)")

// RenderFindingThread renders one finding's forge thread body. Stub until E8-S08.
//
//nolint:revive // spec name RenderFindingThread (E8-S08) — package stutter intentional
func RenderFindingThread(_ Fixture, _ Context) (string, error) {
	return "", ErrFindingThreadNotImplemented
}
