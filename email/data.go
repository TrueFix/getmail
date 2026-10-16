package email

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"iter"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"strings"
)

// MultipartIterator flattens nested multipart messages without decoding leaf
// parts. Consumers must apply Content-Transfer-Encoding themselves.
func MultipartIterator(r io.Reader, boundary string) iter.Seq2[*multipart.Part, error] {
	return multipartIterator(r, boundary, 0)
}

func multipartIterator(r io.Reader, boundary string, depth int) iter.Seq2[*multipart.Part, error] {
	return func(yield func(*multipart.Part, error) bool) {
		if depth > 30 {
			yield(nil, fmt.Errorf("%w: MIME nesting too deep", ErrLimitExceeded))
			return
		}
		mr := multipart.NewReader(r, boundary)
		for {
			part, err := mr.NextRawPart()
			if err == io.EOF {
				return
			}
			if err != nil {
				yield(nil, err)
				return
			}
			ct, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
			if err == nil && strings.HasPrefix(ct, "multipart/") {
				if params["boundary"] == "" {
					yield(nil, fmt.Errorf("missing multipart boundary"))
					return
				}
				for child, childErr := range multipartIterator(part, params["boundary"], depth+1) {
					if !yield(child, childErr) || childErr != nil {
						return
					}
				}
			} else if !yield(part, nil) {
				return
			}
		}
	}
}

func decodingReader(r io.Reader, encoding string) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r), nil
	case "quoted-printable":
		return quotedprintable.NewReader(r), nil
	case "7bit", "8bit", "binary", "":
		return r, nil
	default:
		return nil, fmt.Errorf("unsupported Content-Transfer-Encoding %q", encoding)
	}
}

func decodeData(data []byte, encoding string) ([]byte, error) {
	r, err := decodingReader(bytes.NewReader(data), encoding)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}
