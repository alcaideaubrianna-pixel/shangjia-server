// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotFeature is the golang structure for table youban_bot_feature.
type YoubanBotFeature struct {
	Id          int64       `json:"id"          orm:"id"          ` //
	FeatureKey  string      `json:"featureKey"  orm:"feature_key" ` //
	Name        string      `json:"name"        orm:"name"        ` //
	Command     string      `json:"command"     orm:"command"     ` //
	Description string      `json:"description" orm:"description" ` //
	ConfigJson  string      `json:"configJson"  orm:"config_json" ` //
	Sort        int         `json:"sort"        orm:"sort"        ` //
	Status      int         `json:"status"      orm:"status"      ` //
	CreatedAt   *gtime.Time `json:"createdAt"   orm:"created_at"  ` //
	UpdatedAt   *gtime.Time `json:"updatedAt"   orm:"updated_at"  ` //
	DeletedAt   *gtime.Time `json:"deletedAt"   orm:"deleted_at"  ` //
}
