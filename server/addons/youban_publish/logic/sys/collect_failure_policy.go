package sys

import (
	"errors"
	"strings"
	"time"
)

const collectConfigurationRetryDelay = 5 * time.Minute

var collectProcessingTerminalMessages = []string{
	errProfileMediaValidation.Error(),
	"采集资料标题为空",
	"value too long for type character varying",
}

// Configurations can be repaired without editing the source message. Keep a
// bounded, slow recheck instead of dropping the event or hot-looping the DB.
func collectConfigurationFailure(err error) bool {
	return collectEveryError(err, func(leaf error) bool {
		switch leaf.Error() {
		case "采集规则未配置目标频道", "存在无权操作或不可用的推送频道":
			return true
		}
		return false
	})
}

func collectTerminalProcessingFailure(err error) bool {
	return collectEveryError(err, func(leaf error) bool {
		if isProfileMediaValidationError(leaf) || leaf.Error() == "采集资料标题为空" {
			return true
		}
		// SQLSTATE, rather than arbitrary SQL text, identifies deterministic input
		// overflow. Connection, lock, disk and timeout failures remain retryable.
		state, ok := leaf.(interface{ SQLState() string })
		return ok && state.SQLState() == "22001"
	})
}

// errors.As/Is alone match one child of errors.Join and could hide a temporary
// sibling. Every leaf must match before a branch may stop or delay retries.
func collectEveryError(err error, matches func(error) bool) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !collectEveryError(child, matches) {
				return false
			}
		}
		return true
	}
	if child := errors.Unwrap(err); child != nil {
		return collectEveryError(child, matches)
	}
	return matches(err)
}

// PostgreSQL varchar limits count characters, not UTF-8 bytes. Only the
// derived display title is bounded; the authoritative body remains intact.
func collectProfileTitle(title string) string {
	runes := []rune(strings.TrimSpace(title))
	if len(runes) > 255 {
		return string(runes[:255])
	}
	return string(runes)
}
