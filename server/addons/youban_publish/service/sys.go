package service

import (
	"context"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type ISysPublish interface {
	StartRuntime(ctx context.Context)
	StopRuntime()
	AdminMerchantList(ctx context.Context, in *sysin.MerchantListInp) (list []*sysin.MerchantModel, totalCount int, err error)
	AdminMerchantSave(ctx context.Context, in *sysin.MerchantSaveInp) (err error)
	AdminMerchantDelete(ctx context.Context, in *sysin.MerchantDeleteInp) (err error)
	AdminAccountList(ctx context.Context, in *sysin.AccountListInp) (list []*sysin.AccountModel, totalCount int, err error)
	AdminAccountSave(ctx context.Context, in *sysin.AccountSaveInp) (err error)
	AdminAccountDelete(ctx context.Context, in *sysin.AccountDeleteInp) (err error)
	AdminTaskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error)
	AdminTaskSave(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error)
	AdminTaskSubmit(ctx context.Context, in *sysin.TaskSubmitInp) (err error)
	AdminTaskCancel(ctx context.Context, in *sysin.TaskCancelInp) (err error)
	CurrentAccount(ctx context.Context) (res *sysin.CurrentAccountModel, err error)
	MyTaskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error)
	MyTaskSave(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error)
	MyTaskSubmit(ctx context.Context, in *sysin.TaskSubmitInp) (err error)
	MyTaskCancel(ctx context.Context, in *sysin.TaskCancelInp) (err error)
}

var localSysPublish ISysPublish

func SysPublish() ISysPublish {
	if localSysPublish == nil {
		panic("implement not found for interface ISysPublish, forgot register?")
	}
	return localSysPublish
}

func RegisterSysPublish(i ISysPublish) {
	localSysPublish = i
}
