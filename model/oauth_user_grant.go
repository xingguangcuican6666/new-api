package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OAuthUserGrant records the scopes a user has consented to for a client. It
// lets returning users skip the consent screen when they request no new scopes,
// powers the user's "authorized applications" list, and is the unit revoked when
// a user disconnects an app.
type OAuthUserGrant struct {
	Id        int64  `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex:idx_oauth_grant_user_client;not null"`
	ClientId  string `json:"client_id" gorm:"uniqueIndex:idx_oauth_grant_user_client;type:varchar(64);not null"`
	Scopes    string `json:"scopes" gorm:"type:varchar(512)"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (OAuthUserGrant) TableName() string {
	return "oauth_user_grants"
}

func (g *OAuthUserGrant) GetScopes() []string {
	return strings.Fields(g.Scopes)
}

// GetOAuthUserGrant returns the grant for a user/client pair, or nil when none
// exists (a nil, nil result means "no prior consent").
func GetOAuthUserGrant(userId int, clientId string) (*OAuthUserGrant, error) {
	var grant OAuthUserGrant
	err := DB.Where("user_id = ? AND client_id = ?", userId, clientId).First(&grant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &grant, nil
}

// UpsertOAuthUserGrant records consent for the exact set of granted scopes,
// replacing any previous set so a later consent that drops a scope is honored.
func UpsertOAuthUserGrant(userId int, clientId string, scopes []string) error {
	grant := OAuthUserGrant{
		UserId:   userId,
		ClientId: clientId,
		Scopes:   strings.Join(scopes, " "),
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"scopes", "updated_at"}),
	}).Create(&grant).Error
}

// ListOAuthUserGrants returns every grant a user has, newest updated first.
func ListOAuthUserGrants(userId int) ([]*OAuthUserGrant, error) {
	var grants []*OAuthUserGrant
	err := DB.Where("user_id = ?", userId).Order("updated_at desc").Find(&grants).Error
	return grants, err
}

// DeleteOAuthUserGrant removes a user's consent for a client and, in one
// transaction, revokes every OAuth token issued under it AND deletes the relay
// API keys the application minted for this user via POST /oauth2/keys. Deleting
// those keys is the gateway's own responsibility on disconnect — it is never
// delegated to the application — so a revoked app loses API access immediately.
func DeleteOAuthUserGrant(userId int, clientId string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&OAuthToken{}).
			Where("user_id = ? AND client_id = ? AND revoked = ?", userId, clientId, false).
			Update("revoked", true).Error; err != nil {
			return err
		}
		if _, err := deleteTokensByOAuthClientTx(tx, clientId, &userId); err != nil {
			return err
		}
		return tx.Where("user_id = ? AND client_id = ?", userId, clientId).
			Delete(&OAuthUserGrant{}).Error
	})
}
