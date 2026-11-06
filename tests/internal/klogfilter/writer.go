package klogfilter

import (
	"bytes"
	"io"
)

var lineSeparator = []byte("\n")

// Writer filters verbose client-go tunneling diagnostics from klog output.
type Writer struct {
	output io.Writer
}

// Compile-time check that Writer implements io.Writer.
var _ io.Writer = (*Writer)(nil)

// New returns a Writer that forwards output to the provided writer.
func New(output io.Writer) *Writer {
	return &Writer{output: output}
}

// Write filters klog records from tunneling connection diagnostics and response bodies, then forwards all other
// records.
func (writer *Writer) Write(input []byte) (int, error) {
	filteredLines := make([][]byte, 0, len(input)/32+1)
	filterResponseBody := false

	for line := range bytes.SplitSeq(input, lineSeparator) {
		if isKlogHeader(line) {
			filterResponseBody = isResponseBodyDiagnostic(line) || isResponseVerbDiagnostic(line) ||
				isRequestBodyDiagnostic(line) || isRequestCurlCommandDiagnostic(line)
		}

		if !filterResponseBody && !isTunnelingDiagnostic(line) {
			filteredLines = append(filteredLines, line)
		}
	}

	if len(filteredLines) == 0 {
		return len(input), nil
	}

	filtered := bytes.Join(filteredLines, lineSeparator)

	written, err := writer.output.Write(filtered)

	if written != len(filtered) {
		if err == nil {
			err = io.ErrShortWrite
		}

		return written, err
	}

	return len(input), err
}

// isTunnelingDiagnostic reports whether a klog line header identifies one of client-go's tunneling files.
func isTunnelingDiagnostic(line []byte) bool {
	header, _, ok := bytes.Cut(line, []byte{']'})
	if !ok {
		return false
	}

	return bytes.Contains(header, []byte("tunneling_connection.go:")) ||
		bytes.Contains(header, []byte("tunneling_dialer.go:"))
}

// isKlogHeader reports whether a line starts with a severity letter and date, as produced by klog before the source
// file and message.
func isKlogHeader(line []byte) bool {
	if len(line) < 2 {
		return false
	}

	switch line[0] {
	case 'I', 'W', 'E', 'F':
		return line[1] >= '0' && line[1] <= '9'
	default:
		return false
	}
}

// isResponseBodyDiagnostic reports whether a klog line identifies client-go's response body diagnostic.
func isResponseBodyDiagnostic(line []byte) bool {
	headerEnd := bytes.IndexByte(line, ']')
	if headerEnd < 0 {
		return false
	}

	return bytes.Contains(line[headerEnd:], []byte(`"Response Body" body=`))
}

// isResponseVerbDiagnostic reports whether a klog line identifies client-go's response verb diagnostic.
func isResponseVerbDiagnostic(line []byte) bool {
	headerEnd := bytes.IndexByte(line, ']')
	if headerEnd < 0 {
		return false
	}

	return bytes.Contains(line[headerEnd:], []byte(`"Response" verb=`))
}

// isRequestBodyDiagnostic reports whether a klog line identifies client-go's request body diagnostic.
func isRequestBodyDiagnostic(line []byte) bool {
	headerEnd := bytes.IndexByte(line, ']')
	if headerEnd < 0 {
		return false
	}

	return bytes.Contains(line[headerEnd:], []byte(`"Request Body" body=`))
}

// isRequestCurlCommandDiagnostic reports whether a klog line identifies client-go's request curlCommand diagnostic.
func isRequestCurlCommandDiagnostic(line []byte) bool {
	headerEnd := bytes.IndexByte(line, ']')
	if headerEnd < 0 {
		return false
	}

	return bytes.Contains(line[headerEnd:], []byte(`"Request" curlCommand=`))
}
