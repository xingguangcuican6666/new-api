package setting

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

const (
	DefaultRegistrationGroupOptionKey = "DefaultRegistrationGroup"
	DefaultRegistrationGroup          = "default"
	maxUserGroupLength                = 64
)

var ErrRegistrationGroupUnavailable = errors.New("registration group configuration is unavailable")

type registrationGroupSnapshot struct {
	configured string
	groups     map[string]struct{}
	err        error
}

var registrationGroups atomic.Pointer[registrationGroupSnapshot]

func init() {
	ApplyRegistrationGroupConfiguration(DefaultRegistrationGroup, map[string]float64{
		"default": 1,
		"vip":     1,
		"svip":    1,
	})
}

func ValidateRegistrationGroup(name string, groups map[string]float64) error {
	if !utf8.ValidString(name) {
		return fmt.Errorf("%w: group name must be valid UTF-8", ErrRegistrationGroupUnavailable)
	}
	if name == "" || strings.TrimSpace(name) != name {
		return fmt.Errorf("%w: group name must be non-empty without surrounding whitespace", ErrRegistrationGroupUnavailable)
	}
	if name == "auto" {
		return fmt.Errorf("%w: auto is reserved for token routing", ErrRegistrationGroupUnavailable)
	}
	if utf8.RuneCountInString(name) > maxUserGroupLength || len(name) > maxUserGroupLength {
		return fmt.Errorf("%w: group name exceeds %d characters or bytes", ErrRegistrationGroupUnavailable, maxUserGroupLength)
	}
	if _, ok := groups[name]; !ok {
		return fmt.Errorf("%w: group %q is not configured", ErrRegistrationGroupUnavailable, name)
	}
	return nil
}

func ApplyRegistrationGroupConfiguration(configured string, groups map[string]float64) error {
	err := ValidateRegistrationGroup(configured, groups)
	publishRegistrationGroupConfiguration(configured, groups, err)
	return err
}

func ApplyInvalidRegistrationGroupConfiguration(configured string, groups map[string]float64, err error) {
	if err == nil {
		err = ErrRegistrationGroupUnavailable
	}
	publishRegistrationGroupConfiguration(configured, groups, fmt.Errorf("%w: %v", ErrRegistrationGroupUnavailable, err))
}

func publishRegistrationGroupConfiguration(configured string, groups map[string]float64, err error) {
	groupSet := make(map[string]struct{}, len(groups))
	for group := range groups {
		groupSet[group] = struct{}{}
	}
	registrationGroups.Store(&registrationGroupSnapshot{
		configured: configured,
		groups:     groupSet,
		err:        err,
	})
}

func ResolveRegistrationGroup(explicit string) (string, error) {
	snapshot := registrationGroups.Load()
	if snapshot == nil {
		return "", ErrRegistrationGroupUnavailable
	}
	if snapshot.err != nil {
		return "", snapshot.err
	}
	group := explicit
	if group == "" {
		group = snapshot.configured
	}
	if !utf8.ValidString(group) || group == "" || strings.TrimSpace(group) != group || group == "auto" || utf8.RuneCountInString(group) > maxUserGroupLength || len(group) > maxUserGroupLength {
		return "", fmt.Errorf("%w: invalid group name", ErrRegistrationGroupUnavailable)
	}
	if _, ok := snapshot.groups[group]; !ok {
		return "", fmt.Errorf("%w: group %q is not configured", ErrRegistrationGroupUnavailable, group)
	}
	return group, nil
}

// RegistrationGroupConfiguration exposes the currently published registration
// group snapshot so callers can observe, save, and restore the live setting.
func RegistrationGroupConfiguration() (string, map[string]struct{}, error) {
	snapshot := registrationGroups.Load()
	if snapshot == nil {
		return "", nil, ErrRegistrationGroupUnavailable
	}
	return snapshot.configured, maps.Clone(snapshot.groups), snapshot.err
}
