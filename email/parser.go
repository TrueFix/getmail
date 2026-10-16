package email

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
)

// Limits apply to one message. Attachment size is the decoded size in bytes.
type Limits struct {
	MaxMessageBytes   int64
	MaxAttachments    int
	MaxAttachmentSize int64
	MaxMIMEParts      int
}

func DefaultLimits() Limits {
	return Limits{25 * 1024 * 1024, 10, 10 * 1024 * 1024, 1000}
}

var ErrLimitExceeded = errors.New("message limit exceeded")

func parseHeaders(h map[string][]string) Headers {
	headers := make(Headers)
	for key, values := range h {
		headers[textproto.CanonicalMIMEHeaderKey(key)] = strings.Join(values, "; ")
	}
	return headers
}

func parseEmailUser(input string) (EmailUser, error) {
	a, err := mail.ParseAddress(strings.TrimSpace(input))
	if err != nil {
		return EmailUser{}, err
	}
	return EmailUser{Name: a.Name, Email: a.Address}, nil
}

func parseEmailUsers(input []string) ([]EmailUser, error) {
	var users []EmailUser
	for _, value := range input {
		addresses, err := mail.ParseAddressList(value)
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			users = append(users, EmailUser{Name: address.Name, Email: address.Address})
		}
	}
	return users, nil
}

func parseContentType(value string) (HeaderContentType, error) {
	if value == "" {
		value = "text/plain; charset=us-ascii"
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		return HeaderContentType{}, err
	}
	parts := strings.SplitN(mediaType, "/", 2)
	if len(parts) != 2 {
		return HeaderContentType{}, fmt.Errorf("invalid media type %q", mediaType)
	}
	return HeaderContentType{MediaType: parts[0], SubType: parts[1], Params: Headers(params)}, nil
}

func parseEmailContentHeader(header map[string][]string) (EmailContentHeader, error) {
	h := textproto.MIMEHeader(header)
	ct, err := parseContentType(h.Get("Content-Type"))
	if err != nil {
		return EmailContentHeader{}, fmt.Errorf("invalid Content-Type: %w", err)
	}
	extra := parseHeaders(header)
	delete(extra, "Content-Type")
	delete(extra, "Content-Transfer-Encoding")
	delete(extra, "Mime-Version")
	return EmailContentHeader{
		ContentType: ct, ContentTransferEncoding: h.Get("Content-Transfer-Encoding"),
		MimeVersion: h.Get("Mime-Version"), Extra: extra,
	}, nil
}

func parseMimeHeaderLines(header map[string][]string) (*MimeHeaders, error) {
	h := textproto.MIMEHeader(header)
	content, err := parseEmailContentHeader(header)
	if err != nil {
		return nil, err
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(h.Get("Subject"))
	if err != nil {
		return nil, fmt.Errorf("invalid Subject: %w", err)
	}
	headers := &MimeHeaders{
		Subject: subject, Date: h.Get("Date"), MimeVersion: content.MimeVersion,
		ContentType: content.ContentType, ContentTransferEncoding: content.ContentTransferEncoding,
		Extra: content.Extra,
	}
	if value := h.Get("From"); value != "" {
		headers.From, err = parseEmailUser(value)
		if err != nil {
			return nil, fmt.Errorf("invalid From: %w", err)
		}
	}
	headers.To, err = parseEmailUsers(h.Values("To"))
	if err != nil {
		return nil, fmt.Errorf("invalid To: %w", err)
	}
	headers.Cc, err = parseEmailUsers(h.Values("Cc"))
	if err != nil {
		return nil, fmt.Errorf("invalid Cc: %w", err)
	}
	for _, key := range []string{"From", "To", "Cc", "Subject", "Date"} {
		delete(headers.Extra, key)
	}
	return headers, nil
}

// readBounded detects overflow without trusting advertised sizes or allocating
// based on an attacker-controlled MIME header.
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return b, err
	}
	var extra [1]byte
	n, err := io.ReadFull(r, extra[:])
	if n > 0 {
		return b, ErrLimitExceeded
	}
	if err != nil && err != io.EOF {
		return b, err
	}
	return b, nil
}

type bodyParser struct {
	limits      Limits
	parts       int
	text, html  *EmailContent
	attachments []*EmailContent
}

func (p *bodyParser) parse(header map[string][]string, r io.Reader, depth int) error {
	p.parts++
	if depth > 30 || p.parts > p.limits.MaxMIMEParts {
		return fmt.Errorf("%w: too many MIME parts or nesting levels", ErrLimitExceeded)
	}
	h, err := parseEmailContentHeader(header)
	if err != nil {
		return err
	}
	disposition := ""
	var params map[string]string
	if value := h.Extra.GetString("Content-Disposition"); value != "" {
		disposition, params, err = mime.ParseMediaType(value)
		if err != nil {
			return fmt.Errorf("invalid Content-Disposition: %w", err)
		}
	}
	isAttachment := disposition == "attachment" || params["filename"] != "" || h.ContentType.Params["name"] != ""
	if h.ContentType.MediaType == "multipart" && !isAttachment {
		boundary := h.ContentType.Params["boundary"]
		if boundary == "" {
			return errors.New("multipart content type missing boundary")
		}
		mr := multipart.NewReader(r, boundary)
		for {
			// NextRawPart avoids implicit quoted-printable decoding; all leaf
			// transfer encodings are decoded exactly once below.
			part, err := mr.NextRawPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read multipart: %w", err)
			}
			if part.Header.Get("Content-Type") == "" && h.ContentType.SubType == "digest" {
				part.Header.Set("Content-Type", "message/rfc822")
			}
			if err := p.parse(part.Header, part, depth+1); err != nil {
				return err
			}
			if err := part.Close(); err != nil {
				return err
			}
		}
	}
	isBody := h.ContentType.MediaType == "text" && (h.ContentType.SubType == "plain" || h.ContentType.SubType == "html") && !isAttachment
	limit := p.limits.MaxMessageBytes
	if !isBody {
		if len(p.attachments) >= p.limits.MaxAttachments {
			return fmt.Errorf("%w: maximum %d attachments", ErrLimitExceeded, p.limits.MaxAttachments)
		}
		limit = p.limits.MaxAttachmentSize
	}
	decoded, err := decodingReader(r, h.ContentTransferEncoding)
	if err != nil {
		return err
	}
	data, err := readBounded(decoded, limit)
	if err != nil {
		return fmt.Errorf("read MIME content (limit %d bytes): %w", limit, err)
	}
	content := &EmailContent{R: bytes.NewReader(data), Headers: h, Size: int64(len(data))}
	if !isBody {
		p.attachments = append(p.attachments, content)
	} else if h.ContentType.SubType == "plain" {
		p.text = content
	} else {
		p.html = content
	}
	return nil
}

func parseEmail(r io.Reader) (*Email, error) {
	return parseEmailWithLimits(r, DefaultLimits())
}

func parseEmailWithLimits(r io.Reader, limits Limits) (*Email, error) {
	if limits.MaxMessageBytes <= 0 || limits.MaxAttachmentSize <= 0 || limits.MaxAttachments < 0 || limits.MaxMIMEParts <= 0 {
		return nil, errors.New("invalid parser limits")
	}
	raw, err := readBounded(r, limits.MaxMessageBytes)
	if err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}
	return parseRawEmail(raw, limits)
}

func parseRawEmail(raw []byte, limits Limits) (*Email, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("read message headers: %w", err)
	}
	headers, err := parseMimeHeaderLines(msg.Header)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return nil, err
	}
	p := bodyParser{limits: limits}
	if err := p.parse(msg.Header, bytes.NewReader(body), 0); err != nil {
		return nil, err
	}
	e := NewEmail()
	e.From, e.Subject, e.Headers = headers.From, headers.Subject, headers
	e.Recipients = append(append([]EmailUser{}, headers.To...), headers.Cc...)
	e.Raw, e.Body = bytes.NewReader(raw), bytes.NewReader(body)
	e.BodyText, e.BodyHTML, e.Attachments = p.text, p.html, p.attachments
	return e, nil
}
