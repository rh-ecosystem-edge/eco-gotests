//go:build unit_test

package klogfilter

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterFiltersTunnelingDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal lines pass through",
			input: "I1106 12:00:00.000000    1234 other.go:12] normal message\n",
			want:  "I1106 12:00:00.000000    1234 other.go:12] normal message\n",
		},
		{
			name:  "tunneling connection lines are dropped",
			input: "I1106 12:00:00.000000    1234 tunneling_connection.go:90] write: 4 bytes\n",
			want:  "",
		},
		{
			name:  "tunneling dialer lines are dropped",
			input: "I1106 12:00:00.000000    1234 tunneling_dialer.go:96] Before WebSocket Upgrade Connection...\n",
			want:  "",
		},
		{
			name: "mixed multi-line writes keep non-tunnel lines",
			input: "I1106 12:00:00.000000    1234 other.go:12] first message\n" +
				"I1106 12:00:00.000000    1234 tunneling_connection.go:90] tunnel message\n" +
				"unrelated continuation\n" +
				"I1106 12:00:00.000000    1234 tunneling_dialer.go:106] another tunnel message\n",
			want: "I1106 12:00:00.000000    1234 other.go:12] first message\n" +
				"unrelated continuation\n",
		},
		{
			name: "response body records are dropped",
			input: "I1106 12:00:00.000000    1234 round_trippers.go:632] \"Response\" status=\"200 OK\" headers=<\r\n" +
				"\tContent-Type: application/vnd.kubernetes.protobuf\r\n" +
				" >\r\n" +
				"I1106 12:00:00.000000    1234 type.go:183] \"Response Body\" body=<\r\n" +
				"\t00000000  6b 38 73 00  0a 0d 0a 02  76 31 12 07 50  |k8s.....v1..P|\r\n" +
				" >\r\n" +
				"I1106 12:00:00.000000    1234 other.go:12] next message\n",
			want: "I1106 12:00:00.000000    1234 round_trippers.go:632] \"Response\" status=\"200 OK\" headers=<\r\n" +
				"\tContent-Type: application/vnd.kubernetes.protobuf\r\n" +
				" >\r\n" +
				"I1106 12:00:00.000000    1234 other.go:12] next message\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer

			written, err := New(&output).Write([]byte(test.input))

			require.NoError(t, err)
			assert.Equal(t, len(test.input), written)
			assert.Equal(t, test.want, output.String())
		})
	}
}

func TestWriterReturnsShortWriteError(t *testing.T) {
	t.Parallel()

	writer := New(shortWriter{limit: 4})

	written, err := writer.Write([]byte("I1106 12:00:00.000000    1234 other.go:12] normal message\n"))

	assert.Equal(t, 4, written)
	assert.ErrorIs(t, err, io.ErrShortWrite)
}

type shortWriter struct {
	limit int
}

func (writer shortWriter) Write(input []byte) (int, error) {
	if len(input) <= writer.limit {
		return len(input), nil
	}

	return writer.limit, nil
}
