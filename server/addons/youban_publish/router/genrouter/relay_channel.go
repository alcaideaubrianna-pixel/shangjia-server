package genrouter

import "hotgo/addons/youban_publish/controller/admin/sys"

func init() {
	LoginRequiredRouter = append(LoginRequiredRouter, sys.Publish.RelayChannelList, sys.Publish.RelayChannelSave, sys.Publish.RelayChannelStatus)
}
