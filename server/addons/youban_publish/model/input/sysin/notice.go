package sysin

import "hotgo/internal/model/input/adminin"

type NoticeListInp struct {
	adminin.NoticeListInp
}

type NoticeViewInp struct {
	adminin.NoticeViewInp
}

type NoticeEditInp struct {
	adminin.NoticeEditInp
}

type NoticeDeleteInp struct {
	adminin.NoticeDeleteInp
}

type NoticeMaxSortInp struct {
	adminin.NoticeMaxSortInp
}

type NoticeStatusInp struct {
	adminin.NoticeStatusInp
}

type PullMessagesInp struct {
	adminin.PullMessagesInp
}

type NoticeReadAllInp struct {
	adminin.NoticeReadAllInp
}

type NoticeMessageListInp struct {
	adminin.NoticeMessageListInp
}

type NoticeListModel = adminin.NoticeListModel

type NoticeViewModel = adminin.NoticeViewModel

type NoticeMaxSortModel = adminin.NoticeMaxSortModel

type PullMessagesModel = adminin.PullMessagesModel

type NoticeMessageListModel = adminin.NoticeMessageListModel

type NoticeUpReadInp struct {
	adminin.NoticeUpReadInp
}
