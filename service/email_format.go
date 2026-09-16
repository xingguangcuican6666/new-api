package service

import (
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

// EmailFormatMatches reports whether the email satisfies the administrator's
// custom EmailFormatRegex policy. An empty configuration is a no-op (the
// built-in validator in ValidateAccountEmail still applies); an invalid
// pattern is logged once per value and treated as no constraint so a broken
// configuration cannot lock every account out.
type emailFormatCacheEntry struct {
	raw string
	re  *regexp.Regexp // nil = invalid pattern, no constraint
}

var emailFormatCache atomic.Pointer[emailFormatCacheEntry]

func EmailFormatMatches(email string) bool {
	raw := strings.TrimSpace(common.EmailFormatRegex)
	if raw == "" {
		return true
	}
	entry := emailFormatCache.Load()
	if entry == nil || entry.raw != raw {
		compiled, err := regexp.Compile(raw)
		if err != nil {
			common.SysError(fmt.Sprintf("invalid EmailFormatRegex, ignoring the custom email format policy: %v", err))
			compiled = nil
		}
		entry = &emailFormatCacheEntry{raw: raw, re: compiled}
		emailFormatCache.Store(entry)
	}
	if entry.re == nil {
		return true
	}
	return entry.re.MatchString(email)
}
