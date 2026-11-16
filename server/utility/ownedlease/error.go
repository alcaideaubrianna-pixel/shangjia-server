package ownedlease

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// RedactError removes credentials included by Redis command error wrappers.
// Is preserves sentinel checks without exposing the original error through
// Unwrap, which would let stack printers disclose the command arguments again.
func RedactError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		message = strings.ReplaceAll(message, secret, "[credential redacted]")
		encoded, _ := json.Marshal(secret)
		if len(encoded) > 2 {
			message = strings.ReplaceAll(message, string(encoded[1:len(encoded)-1]), "[credential redacted]")
		}
	}
	return &redactedError{message: message, cause: err}
}

type redactedError struct {
	message string
	cause   error
}

func (e *redactedError) Error() string        { return e.message }
func (e *redactedError) Is(target error) bool { return errors.Is(e.cause, target) }
func (e *redactedError) Format(state fmt.State, verb rune) {
	_, _ = fmt.Fprint(state, e.message)
}
