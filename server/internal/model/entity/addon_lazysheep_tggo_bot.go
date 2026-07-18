// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoBot is the golang structure for table addon_lazysheep_tggo_bot.
type AddonLazysheepTggoBot struct {
	Id            int64       `json:"id"            orm:"id"             description:"主键"`
	BotKey        string      `json:"botKey"        orm:"bot_key"        description:"机器人标识"`
	Role          string      `json:"role"          orm:"role"           description:"机器人角色"`
	MemberId      int64       `json:"memberId"      orm:"member_id"      description:"所属后台用户"`
	Token         string      `json:"token"         orm:"token"          description:"Telegram Bot Token"`
	BotName       string      `json:"botName"       orm:"bot_name"       description:"机器人名称"`
	Username      string      `json:"username"      orm:"username"       description:"Telegram username"`
	WebhookSecret string      `json:"webhookSecret" orm:"webhook_secret" description:"Webhook Secret"`
	WebhookPath   string      `json:"webhookPath"   orm:"webhook_path"   description:"Webhook 路径"`
	Enabled       int         `json:"enabled"       orm:"enabled"        description:"是否启用"`
	AutoPull      int         `json:"autoPull"      orm:"auto_pull"      description:"自动采集"`
	AutoForward   int         `json:"autoForward"   orm:"auto_forward"   description:"自动推送"`
	ReviewEnabled int         `json:"reviewEnabled" orm:"review_enabled" description:"审核开关"`
	AllowVerify   int         `json:"allowVerify"   orm:"allow_verify"   description:"允许查看验证"`
	AllowLocation int         `json:"allowLocation" orm:"allow_location" description:"允许查看位置"`
	MemberVerify  int         `json:"memberVerify"  orm:"member_verify"  description:"验证仅会员"`
	MemberPoints  int         `json:"memberPoints"  orm:"member_points"  description:"积分解锁"`
	SignFollow    int         `json:"signFollow"    orm:"sign_follow"    description:"签到关注校验"`
	SignChannels  *gjson.Json `json:"signChannels"  orm:"sign_channels"  description:"签到必关频道"`
	ReviewText    string      `json:"reviewText"    orm:"review_text"    description:"审核文案模板"`
	PublishText   string      `json:"publishText"   orm:"publish_text"   description:"推送文案模板"`
	Sort          int         `json:"sort"          orm:"sort"           description:"排序"`
	Status        int         `json:"status"        orm:"status"         description:"状态"`
	CreatedBy     int64       `json:"createdBy"     orm:"created_by"     description:"创建者"`
	UpdatedBy     int64       `json:"updatedBy"     orm:"updated_by"     description:"更新者"`
	CreatedAt     *gtime.Time `json:"createdAt"     orm:"created_at"     description:"创建时间"`
	UpdatedAt     *gtime.Time `json:"updatedAt"     orm:"updated_at"     description:"更新时间"`
}
