//go:build unit_test

package stdinmatcher

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeBmcSlow struct {
	text      []byte
	pos       int
	sendEOF   bool
	sleepTime time.Duration
	sleep     bool
}

func newFakeBmcSlow(text string, sleepTime time.Duration, sendEOF bool) *fakeBmcSlow {
	return &fakeBmcSlow{
		text:      []byte(text),
		pos:       0,
		sendEOF:   sendEOF,
		sleepTime: sleepTime,
	}
}

func (b *fakeBmcSlow) OpenSerialConsole(cmd string) (io.Reader, io.WriteCloser, error) {
	return b, nil, nil
}

func (b *fakeBmcSlow) CloseSerialConsole() error {
	return nil
}

func (b *fakeBmcSlow) Read(input []byte) (int, error) {
	if b.sleep {
		time.Sleep(b.sleepTime)
		b.sleep = false
	}

	if b.pos >= len(b.text) {
		if b.sendEOF {
			return 0, io.EOF
		}

		for {
			// Sleep forever to block the reader.
			time.Sleep(1 * time.Second)
		}
	}

	avail := len(b.text) - b.pos
	size := len(input)

	if avail >= size {
		for inti := 0; inti < size; inti++ {
			char := b.text[b.pos+inti]
			input[inti] = char

			if char == '\n' {
				b.sleep = true
				b.pos += inti + 1

				return inti + 1, nil
			}
		}

		b.pos += size
	} else {
		for inti := 0; inti < avail; inti++ {
			char := b.text[b.pos+inti]
			input[inti] = char

			if char == '\n' {
				b.sleep = true
				b.pos += inti + 1

				return inti + 1, nil
			}
		}

		b.pos += avail

		return avail, nil
	}

	return 0, nil
}

//nolint:unused // used by unit tests
type fakeBmc struct {
	serialConsoleText string
}

//nolint:unused // used by unit tests
func (b *fakeBmc) OpenSerialConsole(cmd string) (io.Reader, io.WriteCloser, error) {
	bytesReader := bytes.NewReader([]byte(b.serialConsoleText))

	return bufio.NewReader(bytesReader), nil, nil
}

//nolint:unused // used by unit tests
func (b *fakeBmc) CloseSerialConsole() error {
	return nil
}

func TestWaitForRegex(t *testing.T) {
	const text = `This is the first line.
And this is the second line!
Not it comes the lassst line :)
`

	// Test 1
	bmc1 := newFakeBmcSlow(text, 1*time.Second, false)
	matches := []Matcher{
		{
			Regex: regexp.MustCompile("line"),
			Times: 3,
		},
		{
			Regex: regexp.MustCompile("not there"),
			Times: 1,
		},
	}

	matchIndex, err := WaitForRegex(bmc1, matches, 10*time.Second)
	assert.NoError(t, err)

	if matchIndex == 0 { // if first match found = success
		t.Log("Expected regex found!\n")
	}

	// Test 2
	bmc2 := newFakeBmcSlow(text, 1*time.Second, false)
	matches = []Matcher{
		{
			Regex: regexp.MustCompile("linex"),
			Times: 3,
		},
		{
			Regex: regexp.MustCompile("lassst"),
			Times: 1,
		},
	}

	matchIndex, err = WaitForRegex(bmc2, matches, 10*time.Second)
	assert.NoError(t, err)

	if matchIndex == 1 { // if second match found = success
		t.Log("Expected regex found!\n")
	}
}
