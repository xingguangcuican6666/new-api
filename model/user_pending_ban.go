package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
)

// PendingBanReasonEmailInvalid marks a delayed ban whose fix condition is a
// valid account email; the account is banned when the deadline passes and the
// email still fails validation, and the pending ban is lifted automatically
// once the email becomes valid.
const PendingBanReasonEmailInvalid = "email_invalid"

// SetUserPendingBan arms the delayed ban of an enabled user, overwriting any
// previous pending ban.
func SetUserPendingBan(id int, reason string, deadline int64) error {
	if id <= 0 || reason == "" || deadline <= 0 {
		return errors.New("无效的延迟封禁参数")
	}
	result := DB.Model(&User{}).
		Where("id = ? AND status = ?", id, common.UserStatusEnabled).
		Updates(map[string]any{
			"pending_ban_reason":   reason,
			"pending_ban_deadline": deadline,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("用户不存在或已被禁用")
	}
	return refreshUserPendingBanCache(id)
}

// ClearUserPendingBan resets the pending ban fields without touching the
// user status; it is used both when the admin lifts a pending ban and when
// the deadline converts it into a full ban.
func ClearUserPendingBan(id int) error {
	result := DB.Model(&User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"pending_ban_reason":   "",
			"pending_ban_deadline": 0,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	return refreshUserPendingBanCache(id)
}

// GetPendingBanUsers returns the enabled users that carry a pending ban.
func GetPendingBanUsers() ([]User, error) {
	var users []User
	err := DB.Select("id", "username", "role", "email", "pending_ban_reason", "pending_ban_deadline").
		Where("status = ? AND pending_ban_deadline > 0", common.UserStatusEnabled).
		Find(&users).Error
	return users, err
}

// refreshUserPendingBanCache keeps an existing Redis user snapshot from
// serving a stale pending-ban state; it is best effort.
func refreshUserPendingBanCache(id int) error {
	var user User
	if err := DB.First(&user, id).Error; err != nil {
		return err
	}
	return updateUserCache(user)
}
