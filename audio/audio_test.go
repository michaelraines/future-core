package audio

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- Mock oto player ---

type mockOtoPlayer struct {
	playing bool
	volume  float64
	seekPos int64
	seekErr error
	err     error
}

func (m *mockOtoPlayer) Play()                    { m.playing = true }
func (m *mockOtoPlayer) Pause()                   { m.playing = false }
func (m *mockOtoPlayer) IsPlaying() bool          { return m.playing }
func (m *mockOtoPlayer) SetVolume(volume float64) { m.volume = volume }
func (m *mockOtoPlayer) Volume() float64          { return m.volume }
func (m *mockOtoPlayer) Err() error               { return m.err }

func (m *mockOtoPlayer) Seek(offset int64, _ int) (int64, error) {
	if m.seekErr != nil {
		return 0, m.seekErr
	}
	m.seekPos = offset
	return offset, nil
}

type mockFactory struct {
	lastPlayer *mockOtoPlayer
}

func (f *mockFactory) newPlayer(_ io.Reader) otoPlayer {
	p := &mockOtoPlayer{volume: 1.0}
	f.lastPlayer = p
	return p
}

// --- Context tests ---

func TestNewTestContext(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	require.NotNil(t, ctx)
	require.Equal(t, 48000, ctx.SampleRate())
	require.True(t, ctx.IsReady())
}

func TestContextIsReadyBlocking(t *testing.T) {
	ch := make(chan struct{})
	ctx := &Context{
		ready:      ch,
		sampleRate: 44100,
	}
	require.False(t, ctx.IsReady())

	close(ch)
	require.True(t, ctx.IsReady())
}

func TestCurrentContext(t *testing.T) {
	resetForTesting()

	require.Nil(t, CurrentContext())

	factory := &mockFactory{}
	ctx := newTestContext(factory)
	currentCtxMu.Lock()
	currentCtx = ctx
	currentCtxMu.Unlock()

	require.Equal(t, ctx, CurrentContext())

	// Cleanup
	resetForTesting()
}

func TestContextClose(t *testing.T) {
	resetForTesting()

	factory := &mockFactory{}
	ctx := newTestContext(factory)
	currentCtxMu.Lock()
	currentCtx = ctx
	currentCtxMu.Unlock()

	require.Equal(t, ctx, CurrentContext())

	err := ctx.Close()
	require.NoError(t, err)
	require.Nil(t, CurrentContext())

	// Double close is safe.
	err = ctx.Close()
	require.NoError(t, err)
}

func TestContextNewPlayer(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	src := bytes.NewReader(make([]byte, 100))
	player := ctx.NewPlayer(src)

	require.NotNil(t, player)
	require.NotNil(t, factory.lastPlayer)
}

// --- Player tests ---

func TestPlayerPlayPause(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	src := bytes.NewReader(make([]byte, 100))
	player := ctx.NewPlayer(src)

	require.False(t, player.IsPlaying())
	player.Play()
	require.True(t, player.IsPlaying())
	player.Pause()
	require.False(t, player.IsPlaying())
}

func TestPlayerVolume(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	src := bytes.NewReader(make([]byte, 100))
	player := ctx.NewPlayer(src)

	require.InDelta(t, 1.0, player.Volume(), 1e-9)
	player.SetVolume(0.5)
	require.InDelta(t, 0.5, player.Volume(), 1e-9)
	player.SetVolume(0.0)
	require.InDelta(t, 0.0, player.Volume(), 1e-9)
}

func TestPlayerSetPosition(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	// bytes.Reader implements io.Seeker.
	data := make([]byte, 48000*4) // 1 second at 48000 Hz stereo 16-bit
	src := bytes.NewReader(data)
	player := ctx.NewPlayer(src)

	err := player.SetPosition(500 * time.Millisecond)
	require.NoError(t, err)

	// Check that the mock oto player was seeked.
	mp := factory.lastPlayer
	expected := durationToBytes(500*time.Millisecond, 48000)
	require.Equal(t, expected, mp.seekPos)
}

func TestPlayerSetPositionNoSeeker(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	// Use a plain reader that doesn't implement io.Seeker.
	src := &nonSeekReader{data: make([]byte, 100)}
	player := ctx.NewPlayer(src)

	err := player.SetPosition(100 * time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support seeking")
}

type nonSeekReader struct {
	data []byte
	pos  int
}

func (r *nonSeekReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestPlayerRewind(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	data := make([]byte, 48000*4)
	src := bytes.NewReader(data)
	player := ctx.NewPlayer(src)

	err := player.Rewind()
	require.NoError(t, err)

	mp := factory.lastPlayer
	require.Equal(t, int64(0), mp.seekPos)
}

func TestPlayerClose(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	src := bytes.NewReader(make([]byte, 100))
	player := ctx.NewPlayer(src)
	player.Play()
	require.True(t, player.IsPlaying())

	err := player.Close()
	require.NoError(t, err)
	require.False(t, player.IsPlaying())
}

func TestPlayerErr(t *testing.T) {
	factory := &mockFactory{}
	ctx := newTestContext(factory)

	src := bytes.NewReader(make([]byte, 100))
	player := ctx.NewPlayer(src)

	require.NoError(t, player.Err())
}

// --- Conversion tests ---

func TestDurationToBytes(t *testing.T) {
	tests := []struct {
		name       string
		d          time.Duration
		sampleRate int
		want       int64
	}{
		{"1s@48000", time.Second, 48000, 48000 * 4},
		{"500ms@44100", 500 * time.Millisecond, 44100, 22050 * 4},
		{"zero", 0, 48000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, durationToBytes(tt.d, tt.sampleRate))
		})
	}
}

func TestBytesToDuration(t *testing.T) {
	tests := []struct {
		name       string
		bytes      int64
		sampleRate int
		want       time.Duration
	}{
		{"1s@48000", 48000 * 4, 48000, time.Second},
		{"500ms@44100", 22050 * 4, 44100, 500 * time.Millisecond},
		{"zero", 0, 48000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, bytesToDuration(tt.bytes, tt.sampleRate))
		})
	}
}

// --- InfiniteLoop tests ---

func TestInfiniteLoopRead(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoop(src, int64(len(data)))

	// Read more than one loop's worth.
	buf := make([]byte, 24)
	n, err := io.ReadFull(loop, buf)
	require.NoError(t, err)
	require.Equal(t, 24, n)

	// Should be 3 repetitions of the data.
	require.Equal(t, data, buf[0:8])
	require.Equal(t, data, buf[8:16])
	require.Equal(t, data, buf[16:24])
}

func TestInfiniteLoopWithIntro(t *testing.T) {
	// 4 bytes intro + 4 bytes loop.
	data := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoopWithIntro(src, 4, 4)

	buf := make([]byte, 16)
	n, err := io.ReadFull(loop, buf)
	require.NoError(t, err)
	require.Equal(t, 16, n)

	// First 4 bytes: intro.
	require.Equal(t, []byte{10, 20, 30, 40}, buf[0:4])
	// Next 4 bytes: loop.
	require.Equal(t, []byte{50, 60, 70, 80}, buf[4:8])
	// Next 4 bytes: loop again (looped back).
	require.Equal(t, []byte{50, 60, 70, 80}, buf[8:12])
	// Next 4 bytes: loop again.
	require.Equal(t, []byte{50, 60, 70, 80}, buf[12:16])
}

func TestInfiniteLoopSeekStart(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoop(src, int64(len(data)))

	// Read 4 bytes to advance position.
	buf := make([]byte, 4)
	_, err := io.ReadFull(loop, buf)
	require.NoError(t, err)

	// Seek back to start.
	pos, err := loop.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(0), pos)

	// Read should start from beginning.
	n, err := io.ReadFull(loop, buf)
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte{1, 2, 3, 4}, buf)
}

func TestInfiniteLoopSeekCurrent(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoop(src, int64(len(data)))

	// Read 2 bytes.
	buf := make([]byte, 2)
	_, err := io.ReadFull(loop, buf)
	require.NoError(t, err)

	// Seek forward 2 from current (pos 2 + 2 = 4).
	pos, err := loop.Seek(2, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, int64(4), pos)
}

func TestInfiniteLoopSeekEnd(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoop(src, int64(len(data)))

	// SeekEnd with -4 should be at position 4.
	pos, err := loop.Seek(-4, io.SeekEnd)
	require.NoError(t, err)
	require.Equal(t, int64(4), pos)
}

func TestInfiniteLoopSeekBeforeStart(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoop(src, int64(len(data)))

	_, err := loop.Seek(-1, io.SeekStart)
	require.Error(t, err)
}

func TestInfiniteLoopSeekInvalidWhence(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoop(src, int64(len(data)))

	_, err := loop.Seek(0, 99)
	require.Error(t, err)
}

func TestInfiniteLoopSeekPastEnd(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoop(src, int64(len(data)))

	// Seeking past the end should wrap around.
	pos, err := loop.Seek(10, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(2), pos) // 10 % 8 = 2
}

func TestInfiniteLoopZeroLength(t *testing.T) {
	data := []byte{}
	src := bytes.NewReader(data)
	loop := NewInfiniteLoop(src, 0)

	buf := make([]byte, 4)
	_, err := loop.Read(buf)
	require.Error(t, err)
}

func TestBytesPerSample(t *testing.T) {
	require.Equal(t, 4, bytesPerSample)
}
