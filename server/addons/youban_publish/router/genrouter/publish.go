package genrouter

import "hotgo/addons/youban_publish/controller/admin/sys"

func init() {
	LoginRequiredRouter = append(LoginRequiredRouter, sys.Publish, sys.Config)
}
