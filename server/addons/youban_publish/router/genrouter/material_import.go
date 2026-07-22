package genrouter

import "hotgo/addons/youban_publish/controller/admin/sys"

func init() {
	AdminRequiredRouter = append(AdminRequiredRouter, sys.MaterialImport)
}
