package genrouter

import "hotgo/addons/youban_feiniu_sync/controller/admin/sys"

func init() { LoginRequiredRouter = append(LoginRequiredRouter, sys.Sync) }
