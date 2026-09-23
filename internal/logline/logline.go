// Package logline defines the shape of an axio log line, shared by the logger
// that writes it and by axio render, which reads it back: the keys a JSON line
// carries, the audit trailer that closes an audited line, the encoder settings
// of each format, and the rendering of a JSON line into the Console's text.
package logline

import (
	"bytes"
	"regexp"
)

// Keys an axio JSON line carries besides the caller's own fields.
const (
	TimeKey         = "timestamp"
	LevelKey        = "level"
	MessageKey      = "message"
	LoggerKey       = "logger"
	CallerKey       = "caller"
	StacktraceKey   = "stacktrace"
	EventKey        = "event"
	ServiceKey      = "service"
	DeploymentKey   = "deployment"
	PreviousHashKey = "previous_hash"
	HashKey         = "hash"
)

// ShortHashLength is how many leading characters of a hash the text format shows.
const ShortHashLength = 6

// ShortHash returns the leading characters of hash that the text format shows.
// The text format cannot be verified, so a hash there only has to be long
// enough to find the same entry in the JSON output.
func ShortHash(hash string) string {
	if len(hash) <= ShortHashLength {
		return hash
	}
	return hash[:ShortHashLength]
}

// AppendTrailer closes body with the audit trailer and a line ending. The
// trailer comes last so that everything before it is exactly the body that
// was hashed. body is left untouched.
func AppendTrailer(body []byte, previousHash, hash string) []byte {
	line := make([]byte, 0, len(body)+len(previousHash)+len(hash)+len(`,"previous_hash":"","hash":""}`)+1)
	line = append(line, body...)
	line = append(line, `,"`+PreviousHashKey+`":"`...)
	line = append(line, previousHash...)
	line = append(line, `","`+HashKey+`":"`...)
	line = append(line, hash...)
	line = append(line, "\"}\n"...)
	return line
}

// SplitTrailer separates an audited line into the body that was hashed and the
// two hashes of its trailer. It reports false when the line does not end with
// a trailer written by [AppendTrailer].
func SplitTrailer(line []byte) (body []byte, previousHash, hash string, ok bool) {
	match := trailerPattern.FindSubmatchIndex(bytes.TrimRight(line, "\r\n"))
	if match == nil {
		return nil, "", "", false
	}
	return line[:match[3]], string(line[match[4]:match[5]]), string(line[match[6]:match[7]]), true
}

// Body returns an encoded JSON object without its closing brace and line
// ending: the bytes an audited line hashes.
func Body(encoded []byte) ([]byte, bool) {
	object := bytes.TrimRight(encoded, "\r\n")
	if len(object) < 2 || object[0] != '{' || object[len(object)-1] != '}' {
		return nil, false
	}
	return object[:len(object)-1], true
}

// trailerPattern matches a whole audited line. The body group is greedy, so
// the trailer it leaves is the last one on the line; a trailer quoted inside a
// string value cannot match, because its quotes are escaped.
var trailerPattern = regexp.MustCompile(`(?s)^(\{.*),"` + PreviousHashKey + `":"((?:[0-9a-f]{64})?)","` + HashKey + `":"([0-9a-f]{64})"\}$`)
