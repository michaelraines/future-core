package audio

import (
	"errors"
	"io"
)

// InfiniteLoop wraps an io.ReadSeeker and loops its content indefinitely.
// It implements io.ReadSeeker so it can be passed directly to Context.NewPlayer.
type InfiniteLoop struct {
	src      io.ReadSeeker
	introLen int64
	loopLen  int64
	pos      int64
	totalLen int64 // introLen + loopLen
	hasIntro bool
}

// NewInfiniteLoop creates an InfiniteLoop that loops the entire source.
// length is the total byte length of the audio data in src.
func NewInfiniteLoop(src io.ReadSeeker, length int64) *InfiniteLoop {
	return &InfiniteLoop{
		src:      src,
		loopLen:  length,
		totalLen: length,
	}
}

// NewInfiniteLoopWithIntro creates an InfiniteLoop that plays an intro section
// once, then loops the loop section indefinitely. introLength is the byte
// length of the non-repeating intro; loopLength is the byte length of the
// repeating section that follows.
func NewInfiniteLoopWithIntro(src io.ReadSeeker, introLength, loopLength int64) *InfiniteLoop {
	return &InfiniteLoop{
		src:      src,
		introLen: introLength,
		loopLen:  loopLength,
		totalLen: introLength + loopLength,
		hasIntro: true,
	}
}

// Read implements io.Reader. When the end of the loop region is reached,
// it seeks back to the loop start and continues reading.
func (l *InfiniteLoop) Read(p []byte) (int, error) {
	if l.loopLen <= 0 {
		return 0, errors.New("audio: InfiniteLoop has zero loop length")
	}

	totalRead := 0
	for len(p) > 0 {
		// Calculate remaining bytes until end of loop region.
		remaining := l.totalLen - l.pos
		if remaining <= 0 {
			// Seek back to loop start.
			loopStart := l.introLen
			if !l.hasIntro {
				loopStart = 0
			}
			if _, err := l.src.Seek(loopStart, io.SeekStart); err != nil {
				return totalRead, err
			}
			l.pos = loopStart
			remaining = l.totalLen - l.pos
		}

		// Limit read to remaining bytes in this iteration.
		toRead := len(p)
		if int64(toRead) > remaining {
			toRead = int(remaining)
		}

		n, err := l.src.Read(p[:toRead])
		totalRead += n
		l.pos += int64(n)
		p = p[n:]

		if errors.Is(err, io.EOF) {
			// Source ended (possibly shorter than declared length).
			// Seek back to loop start to continue filling the buffer.
			loopStart := l.introLen
			if !l.hasIntro {
				loopStart = 0
			}
			if _, seekErr := l.src.Seek(loopStart, io.SeekStart); seekErr != nil {
				return totalRead, seekErr
			}
			l.pos = loopStart
			continue
		}
		if err != nil {
			return totalRead, err
		}
	}
	return totalRead, nil
}

// Seek implements io.Seeker. The seek position is relative to the start
// of the audio data (including intro if present).
func (l *InfiniteLoop) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = l.pos + offset
	case io.SeekEnd:
		// For an infinite loop, SeekEnd is relative to totalLen.
		newPos = l.totalLen + offset
	default:
		return 0, errors.New("audio: invalid seek whence")
	}

	if newPos < 0 {
		return 0, errors.New("audio: seek before start")
	}

	// Normalize position within the loop region.
	if newPos >= l.totalLen && l.loopLen > 0 {
		loopStart := l.introLen
		if !l.hasIntro {
			loopStart = 0
		}
		excess := newPos - l.totalLen
		newPos = loopStart + (excess % l.loopLen)
	}

	if _, err := l.src.Seek(newPos, io.SeekStart); err != nil {
		return 0, err
	}
	l.pos = newPos
	return l.pos, nil
}
