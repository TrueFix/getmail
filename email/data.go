package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"iter"
	"mime/multipart"
	"mime/quotedprintable"
	"strings"
)

func MultipartIterator(r io.Reader, boundary string) iter.Seq2[*multipart.Part, error] {
	multipartReader := multipart.NewReader(r, boundary)

	return func(yield func(*multipart.Part, error) bool) {
		for {
			part, err := multipartReader.NextPart()
			if err != nil {
				if err != io.EOF {
					yield(nil, err)
				}
				break
			}

			header := parseHeaders(part.Header)
			contentType := header.GetString("Content-Type")

			// Handle nested multipart
			if strings.Contains(contentType, "multipart") {
				subBoundary := header.GetParam("Content-Type", "boundary")

				if subBoundary != "" {
					subParts := MultipartIterator(part, subBoundary)
					for subPart, subErr := range subParts {
						if subErr != nil {
							yield(nil, subErr)
							return
						}
						if !yield(subPart, nil) {
							return
						}
					}
					continue // Skip yielding parent multipart
				}
			}

			if !yield(part, nil) {
				return
			}
		}
	}
}

func splitAndTrim(value, sep string) []string {
	parts := strings.Split(value, sep)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func decodeData(data []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "base64":
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
		n, err := base64.StdEncoding.Decode(decoded, data)
		if err != nil {
			return nil, err
		}
		return decoded[:n], nil
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(data))
		return io.ReadAll(reader)
	case "7bit", "8bit", "binary", "":
		return data, nil
	default:
		LogWarning("decodeData", fmt.Sprintf("Unknown encoding %s, returning raw data", encoding))
		return data, nil // Unknown encoding
	}
}
