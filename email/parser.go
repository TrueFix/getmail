package email

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"net/textproto"
	"regexp"
	"strings"
)

func parseHeaders(h map[string][]string) Headers {
	headers := make(Headers)
	for k, v := range h {
		if len(v) == 0 {
			continue
		}
		if len(v) == 1 {
			v = splitAndTrim(v[0], ";")
		}
		headers[textproto.CanonicalMIMEHeaderKey(k)] = strings.Join(v, "; ")
	}
	return headers
}

func parseEmailUser(input string) (EmailUser, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return EmailUser{}, fmt.Errorf("input cannot be empty")
	}

	// Remove "TO:" prefix if present
	if strings.HasPrefix(strings.ToUpper(input), "TO:") {
		input = strings.TrimSpace(input[3:])
	}

	// Common patterns to match:
	// 1. "Name" <email@domain>
	// 2. Name <email@domain>
	// 3. <email@domain>
	// 4. email@domain

	// Try #1 and #2
	re1 := regexp.MustCompile(`^"?([^"<@]+)?"?\s*<([^<>@\s]+@[^<>@\s]+)>$`)
	if matches := re1.FindStringSubmatch(input); len(matches) == 3 {
		name := strings.Trim(matches[1], `" `)
		email := strings.TrimSpace(matches[2])
		return EmailUser{Name: name, Email: email}, nil
	}

	// Try #3
	re2 := regexp.MustCompile(`^<([^<>@\s]+@[^<>@\s]+)>$`)
	if matches := re2.FindStringSubmatch(input); len(matches) == 2 {
		return EmailUser{Name: "", Email: strings.TrimSpace(matches[1])}, nil
	}

	// Try #4 (just plain email)
	re3 := regexp.MustCompile(`^([^<>@\s]+@[^<>@\s]+)$`)
	if matches := re3.FindStringSubmatch(input); len(matches) == 2 {
		return EmailUser{Name: "", Email: strings.TrimSpace(matches[1])}, nil
	}

	return EmailUser{}, fmt.Errorf("could not parse email line: %s", input)
}

func parseEmailUsers(input []string) ([]EmailUser, error) {
	var users []EmailUser
	for _, line := range input {
		email, err := parseEmailUser(line)
		if err != nil {
			return nil, err
		}
		users = append(users, email)
	}
	return users, nil
}

func parseContentType(value string) (HeaderContentType, error) {
	var ct HeaderContentType
	ct.Params = make(Headers)

	parts := strings.Split(value, ";")
	media := strings.SplitN(strings.TrimSpace(parts[0]), "/", 2)
	if len(media) != 2 {
		return ct, fmt.Errorf("invalid media type: %s", value)
	}
	ct.MediaType = strings.TrimSpace(media[0])
	ct.SubType = strings.TrimSpace(media[1])

	for _, param := range parts[1:] {
		pair := strings.SplitN(param, "=", 2)
		if len(pair) != 2 {
			LogInfo("parseContentType", fmt.Sprintf("malformed parameter in header: %q", param))
			continue
		}
		key := strings.TrimSpace(pair[0])
		val := strings.Trim(strings.TrimSpace(pair[1]), `"`)
		ct.Params[key] = val
	}

	return ct, nil
}

func parseEmailContentHeader(header map[string][]string) (EmailContentHeader, error) {
	headers := EmailContentHeader{
		Extra: make(Headers),
	}

	lines := []string{}
	for k, v := range header {
		lines = append(lines, fmt.Sprintf("%s: %s", k, strings.Join(v, "; ")))
	}

	for _, line := range lines {
		line := strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)

		if len(parts) != 2 {
			LogInfo("parseEmailContentHeader", fmt.Sprintf("Skipping malformed header line: %s, Length: %d", line, len(parts)))
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch textproto.CanonicalMIMEHeaderKey(key) {
		case "Content-Transfer-Encoding":
			headers.ContentTransferEncoding = value
		case "Content-Type":
			// Parse Content-Type and parameters
			ct, err := parseContentType(value)
			if err != nil {
				LogInfo("parseEmailContentHeader", fmt.Sprintf("Skipping malformed Content-Type header: %s", value))
				continue
			}
			headers.ContentType = ct
		case "Mime-Version":
			headers.MimeVersion = value
		default:
			headers.Extra[key] = value
		}
	}
	return headers, nil
}

func parseMimeHeaderLines(header map[string][]string) (*MimeHeaders, error) {
	headers := MimeHeaders{
		Extra: make(Headers),
	}

	lines := []string{}
	for k, v := range header {
		lines = append(lines, fmt.Sprintf("%s: %s", k, strings.Join(v, "; ")))
	}

	for _, line := range lines {
		line := strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)

		if len(parts) != 2 {
			LogInfo("parseMimeHeaderLines", fmt.Sprintf("Skipping malformed header line: %s, Length: %d", line, len(parts)))
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch textproto.CanonicalMIMEHeaderKey(key) {
		case "Mime-Version":
			headers.MimeVersion = value
		case "Date":
			headers.Date = value
		case "Subject":
			headers.Subject = value
		case "From":
			fromUser, err := parseEmailUser(value)
			if err != nil {
				return nil, errors.New("smtp: error parsing from header")
			}
			headers.From = fromUser

		case "To":
			toUsers, err := parseEmailUsers(strings.Split(value, ","))
			if err != nil {
				return nil, errors.New("smtp: error parsing to header")
			}
			headers.To = toUsers

		case "Cc":
			ccUsers, err := parseEmailUsers(strings.Split(value, ","))
			if err != nil {
				return nil, errors.New("smtp: error parsing cc header")
			}
			headers.Cc = ccUsers

		case "Content-Transfer-Encoding":
			headers.ContentTransferEncoding = value

		case "Content-Type":
			// Parse Content-Type and parameters
			ct, err := parseContentType(value)
			if err != nil {
				LogInfo("parseMimeHeaderLines", fmt.Sprintf("Skipping malformed Content-Type header: %s", value))
				continue
			}
			headers.ContentType = ct

		default:
			headers.Extra[key] = value
		}

	}

	return &headers, nil
}

func parseBody(h *MimeHeaders, b []byte) (*EmailContent, *EmailContent, []*EmailContent, error) {
	var (
		bodyText    *EmailContent
		bodyHTML    *EmailContent
		attachments []*EmailContent
	)

	if strings.HasPrefix(h.ContentType.MediaType, "multipart") {
		boundary := h.ContentType.Params["boundary"]
		if boundary == "" {
			return nil, nil, nil, errors.New("multipart content type missing boundary parameter")
		}

		for part, err := range MultipartIterator(bytes.NewReader(b), boundary) {
			if err != nil {
				LogInfo("parseBody", fmt.Sprintf("error iterating multipart: %v", err))
				continue
			}
			h, err := parseEmailContentHeader(part.Header)
			if err != nil {
				LogInfo("parseBody", fmt.Sprintf("error parsing part headers %v", err))
				continue
			}
			data, err := io.ReadAll(part)
			if err != nil {
				LogInfo("parseBody", fmt.Sprintf("error reading part data %v", err))
				continue
			}

			if h.ContentTransferEncoding != "" {
				data, err = decodeData(data, h.ContentTransferEncoding)
				if err != nil {
					LogInfo("parseBody", fmt.Sprintf("error decoding part data %v", err))
					continue
				}
			}

			// LogInfo("parseBody", fmt.Sprintf("Part content type: %s/%s, size: %d bytes", h.ContentType.MediaType, h.ContentType.SubType, len(data)))

			if len(part.Header) == 0 {
				LogInfo("parseBody", "skipping part with empty headers")
				continue
			}

			if h.ContentType.MediaType == "text" && h.ContentType.SubType == "plain" {
				bodyText = &EmailContent{
					R:       bytes.NewReader(data),
					Headers: h,
					Size:    int64(len(data)),
				}
			} else if h.ContentType.MediaType == "text" && h.ContentType.SubType == "html" {
				bodyHTML = &EmailContent{
					R:       bytes.NewReader(data),
					Headers: h,
					Size:    int64(len(data)),
				}
			} else {
				attachments = append(attachments, &EmailContent{
					R:       bytes.NewReader(data),
					Headers: h,
					Size:    int64(len(data)),
				})
			}
		}
	} else if h.ContentType.MediaType == "text" && h.ContentType.SubType == "plain" {
		bodyText = &EmailContent{
			R:       bytes.NewReader(b),
			Headers: EmailContentHeader{},
			Size:    int64(len(b)),
		}
	} else if h.ContentType.MediaType == "text" && h.ContentType.SubType == "html" {
		bodyHTML = &EmailContent{
			R:       bytes.NewReader(b),
			Headers: EmailContentHeader{},
			Size:    int64(len(b)),
		}
	} else {
		LogInfo("parseBody", fmt.Sprintf("Unhandled singlepart content type: %s/%s", h.ContentType.MediaType, h.ContentType.SubType))
		return nil, nil, nil, fmt.Errorf("unhandled singlepart content type: %s/%s", h.ContentType.MediaType, h.ContentType.SubType)
	}

	return bodyText, bodyHTML, attachments, nil

}

func parseEmail(r io.Reader) (*Email, error) {
	rawEmail, err := io.ReadAll(r)
	// LogInfo("parseEmail", fmt.Sprintf("[Raw Email Data]\n\n%s\n\n", string(rawEmail)))
	if err != nil {
		return nil, fmt.Errorf("parseEmail: failed to read raw email: %w", err)
	}

	msg, err := mail.ReadMessage(bytes.NewReader(rawEmail))
	if err != nil {
		return nil, fmt.Errorf("parseEmail: failed to read message: %w", err)
	}

	hdr, bdy := msg.Header, msg.Body
	bdyBytes, err := io.ReadAll(bdy)
	if err != nil {
		return nil, fmt.Errorf("parseEmail: failed to read body: %w", err)
	}

	// Parse headers
	headers, err := parseMimeHeaderLines(hdr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse headers: %w", err)
	}

	// Combine recipients (cc + to + from) into a flat list
	recipients := append([]EmailUser{}, headers.Cc...)
	recipients = append(recipients, headers.To...)

	// Parse body based on Content-Type
	bt, bh, atts, err := parseBody(headers, bdyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse body: %w", err)
	}

	// Construct the Email object
	email := NewEmail()
	email.From = headers.From
	email.RcptTo = recipients
	email.Subject = headers.Subject

	email.Headers = headers

	email.Raw = bytes.NewReader(rawEmail)
	email.Body = bytes.NewReader(bdyBytes)

	email.BodyText = bt
	email.BodyHTML = bh

	email.Attachments = atts

	return email, nil
}
