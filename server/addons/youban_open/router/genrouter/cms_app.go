package genrouter

import "hotgo/addons/youban_open/controller/admin/sys"

func init() { LoginRequiredRouter = append(LoginRequiredRouter, sys.CmsApp) }
