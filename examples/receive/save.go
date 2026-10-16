package main

import (
	"errors"
	"io"
	"os"

	"github.com/TrueFix/getmail/email"
)

// saveRawEmail can be called from receiveEmail to store the original message.
func saveRawEmail(message *email.Email, directory string) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, message.ID+"-*.eml")
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, message.Raw)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(file.Name())
		return err
	}
	return nil
}
