// Package pcm provides shared PCM audio utilities for audio decoders.
package pcm

import "encoding/binary"

// Resample performs linear interpolation resampling from srcRate to dstRate.
// Input and output are stereo 16-bit signed LE PCM (4 bytes per frame).
func Resample(data []byte, srcRate, dstRate int) []byte {
	srcFrames := len(data) / 4
	if srcFrames < 2 {
		return data
	}

	ratio := float64(srcRate) / float64(dstRate)
	dstFrames := int(float64(srcFrames) / ratio)
	out := make([]byte, dstFrames*4)

	for i := 0; i < dstFrames; i++ {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx >= srcFrames-1 {
			srcIdx = srcFrames - 2
			frac = 1.0
		}
		if srcIdx < 0 {
			srcIdx = 0
			frac = 0
		}

		// Left channel
		l0 := int16(binary.LittleEndian.Uint16(data[srcIdx*4:]))
		l1 := int16(binary.LittleEndian.Uint16(data[(srcIdx+1)*4:]))
		left := int16(float64(l0) + frac*(float64(l1)-float64(l0)))

		// Right channel
		r0 := int16(binary.LittleEndian.Uint16(data[srcIdx*4+2:]))
		r1 := int16(binary.LittleEndian.Uint16(data[(srcIdx+1)*4+2:]))
		right := int16(float64(r0) + frac*(float64(r1)-float64(r0)))

		binary.LittleEndian.PutUint16(out[i*4:], uint16(left))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(right))
	}
	return out
}
