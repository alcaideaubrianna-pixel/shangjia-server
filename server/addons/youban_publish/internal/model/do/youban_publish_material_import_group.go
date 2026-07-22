// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMaterialImportGroup is the golang structure of table hg_youban_publish_material_import_group for DAO operations like Where/Data.
type YoubanPublishMaterialImportGroup struct {
	g.Meta           `orm:"table:hg_youban_publish_material_import_group, do:true"`
	Id               any         //
	TaskId           any         //
	TenantId         any         //
	AccountId        any         //
	SourceChatId     any         //
	SourceGroupedId  any         //
	SourceMessageIds any         //
	SourceUniqueKey  any         //
	Title            any         //
	Nickname         any         //
	ProfileNo        any         //
	RawText          any         //
	ProfileText      any         //
	VerifyText       any         //
	MediaJson        any         //
	MediaTotal       any         //
	MediaDone        any         //
	MediaFailed      any         //
	ProfileId        any         //
	TaskProfileId    any         //
	Status           any         //
	ErrorMessage     any         //
	MessageAt        *gtime.Time //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
}
