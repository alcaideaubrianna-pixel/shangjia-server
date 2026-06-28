package sys

import (
	"context"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/contexts"
)

type sSysPublish struct {
	runtimeCancel context.CancelFunc
	runtimeDone   chan struct{}
	runtimeMu     publishRuntimeMutex
	telegramBotMu publishRuntimeMutex
	telegramBots  map[string]*tgbot.Bot
	tgLoginMu     publishRuntimeMutex
	tgLogins      map[string]*telegramLoginRuntime
}

func NewSysPublish() *sSysPublish {
	return &sSysPublish{
		tgLogins: make(map[string]*telegramLoginRuntime),
	}
}

func init() {
	service.RegisterSysPublish(NewSysPublish())
}

func (s *sSysPublish) AdminMerchantList(ctx context.Context, in *sysin.MerchantListInp) (list []*sysin.MerchantModel, totalCount int, err error) {
	merchantColumns := pdao.YoubanPublishMerchant.Columns()
	mod := pdao.YoubanPublishMerchant.Ctx(ctx).WhereNull(merchantColumns.DeletedAt)
	if in.Status > 0 {
		mod = mod.Where(merchantColumns.Status, in.Status)
	}
	kw := strings.TrimSpace(in.Keyword)
	if kw != "" {
		mod = mod.WhereLike(merchantColumns.Name, "%"+kw+"%").WhereOrLike(merchantColumns.ContactName, "%"+kw+"%")
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取商家总数失败")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc(merchantColumns.Id).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取商家列表失败")
	}
	return
}

func (s *sSysPublish) AdminMerchantSave(ctx context.Context, in *sysin.MerchantSaveInp) (err error) {
	if err = in.Filter(ctx); err != nil {
		return
	}
	merchantColumns := pdao.YoubanPublishMerchant.Columns()
	data := g.Map{
		merchantColumns.Name:         in.Name,
		merchantColumns.ContactName:  strings.TrimSpace(in.ContactName),
		merchantColumns.ContactPhone: strings.TrimSpace(in.ContactPhone),
		merchantColumns.Remark:       strings.TrimSpace(in.Remark),
		merchantColumns.Status:       in.Status,
		merchantColumns.UpdatedBy:    contexts.GetUserId(ctx),
		merchantColumns.UpdatedAt:    gtime.Now(),
	}
	if in.Id > 0 {
		_, err = pdao.YoubanPublishMerchant.Ctx(ctx).Where(merchantColumns.Id, in.Id).WhereNull(merchantColumns.DeletedAt).Data(data).Update()
	} else {
		data[merchantColumns.CreatedBy] = contexts.GetUserId(ctx)
		data[merchantColumns.CreatedAt] = gtime.Now()
		_, err = pdao.YoubanPublishMerchant.Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存商家失败")
	}
	return nil
}

func (s *sSysPublish) AdminMerchantDelete(ctx context.Context, in *sysin.MerchantDeleteInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	merchantColumns := pdao.YoubanPublishMerchant.Columns()
	_, err = pdao.YoubanPublishMerchant.Ctx(ctx).WhereIn(merchantColumns.Id, in.Ids).Data(g.Map{
		merchantColumns.DeletedBy: contexts.GetUserId(ctx),
		merchantColumns.DeletedAt: gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除商家失败")
	}
	return nil
}

func (s *sSysPublish) AdminAccountList(ctx context.Context, in *sysin.AccountListInp) (list []*sysin.AccountModel, totalCount int, err error) {
	return s.accountList(ctx, in)
}

func (s *sSysPublish) AdminAccountSave(ctx context.Context, in *sysin.AccountSaveInp) (err error) {
	if err = in.Filter(ctx); err != nil {
		return
	}
	if err = s.ensureMerchant(ctx, in.MerchantId); err != nil {
		return
	}
	if err = s.prepareAdminMemberBinding(ctx, in); err != nil {
		return err
	}
	if err = s.prepareAccountParent(ctx, in); err != nil {
		return err
	}
	memberId, err := s.ensureAdminMemberForAccount(ctx, in)
	if err != nil {
		return err
	}
	in.AdminMemberId = memberId
	accountColumns := pdao.YoubanPublishAccount.Columns()
	data := g.Map{
		accountColumns.MerchantId:         in.MerchantId,
		accountColumns.AdminMemberId:      in.AdminMemberId,
		accountColumns.ParentId:           in.ParentId,
		accountColumns.AccountType:        in.AccountType,
		accountColumns.Nickname:           strings.TrimSpace(in.Nickname),
		accountColumns.Username:           strings.TrimSpace(in.Username),
		accountColumns.TelegramUserId:     strings.TrimSpace(in.TelegramUserId),
		accountColumns.TelegramUsername:   strings.TrimSpace(in.TelegramUsername),
		accountColumns.DailyPublishLimit:  in.DailyPublishLimit,
		accountColumns.CanDirectPublish:   in.CanDirectPublish,
		accountColumns.AllowedChannelJson: strings.TrimSpace(in.AllowedChannelJson),
		accountColumns.AllowedRegionJson:  strings.TrimSpace(in.AllowedRegionJson),
		accountColumns.Remark:             strings.TrimSpace(in.Remark),
		accountColumns.Status:             in.Status,
		accountColumns.UpdatedBy:          contexts.GetUserId(ctx),
		accountColumns.UpdatedAt:          gtime.Now(),
	}
	if in.Id > 0 {
		_, err = pdao.YoubanPublishAccount.Ctx(ctx).Where(accountColumns.Id, in.Id).WhereNull(accountColumns.DeletedAt).Data(data).Update()
	} else {
		data[accountColumns.CreatedBy] = contexts.GetUserId(ctx)
		data[accountColumns.CreatedAt] = gtime.Now()
		_, err = pdao.YoubanPublishAccount.Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存上架账号失败")
	}
	return nil
}

func (s *sSysPublish) AdminAccountDelete(ctx context.Context, in *sysin.AccountDeleteInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		memberIds, err := s.adminMemberIdsForAccounts(ctx, tx, in.Ids)
		if err != nil {
			return err
		}
		accountColumns := pdao.YoubanPublishAccount.Columns()
		if _, err = tx.Model(pdao.YoubanPublishAccount.Table()).Safe().Ctx(ctx).WhereIn(accountColumns.Id, in.Ids).Data(g.Map{
			accountColumns.DeletedBy: contexts.GetUserId(ctx),
			accountColumns.DeletedAt: gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrap(err, "删除上架账号失败")
		}
		return s.disableAdminMembers(ctx, tx, memberIds)
	})
}

func (s *sSysPublish) AdminTaskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error) {
	return s.taskList(ctx, in)
}

func (s *sSysPublish) AdminTaskSave(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	return s.saveTask(ctx, in)
}

func (s *sSysPublish) AdminTaskSubmit(ctx context.Context, in *sysin.TaskSubmitInp) (err error) {
	return s.submitTask(ctx, in.Id, 0)
}

func (s *sSysPublish) AdminTaskCancel(ctx context.Context, in *sysin.TaskCancelInp) (err error) {
	return s.cancelTask(ctx, in.Id, 0)
}

func (s *sSysPublish) CurrentAccount(ctx context.Context) (res *sysin.CurrentAccountModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return &sysin.CurrentAccountModel{AccountModel: account}, nil
}

func (s *sSysPublish) MyTaskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	in.MerchantId = account.MerchantId
	in.AccountId = account.Id
	return s.taskList(ctx, in)
}

func (s *sSysPublish) MyTaskSave(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return 0, err
	}
	in.MerchantId = account.MerchantId
	in.AccountId = account.Id
	return s.saveTask(ctx, in)
}

func (s *sSysPublish) MyTaskSubmit(ctx context.Context, in *sysin.TaskSubmitInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.submitTask(ctx, in.Id, account.Id)
}

func (s *sSysPublish) MyTaskCancel(ctx context.Context, in *sysin.TaskCancelInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.cancelTask(ctx, in.Id, account.Id)
}

func (s *sSysPublish) accountList(ctx context.Context, in *sysin.AccountListInp) (list []*sysin.AccountModel, totalCount int, err error) {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	mod := pdao.YoubanPublishAccount.DB().Model(pdao.YoubanPublishAccount.Table()+" a").Safe().Ctx(ctx).
		LeftJoin(publishMerchantTable+" m", "m.id=a.merchant_id").
		WhereNull("a." + accountColumns.DeletedAt).
		Fields("a.*,m.name AS merchant_name")
	if in.MerchantId > 0 {
		mod = mod.Where("a."+accountColumns.MerchantId, in.MerchantId)
	}
	if in.AccountType != "" {
		mod = mod.Where("a."+accountColumns.AccountType, in.AccountType)
	}
	if in.Status > 0 {
		mod = mod.Where("a."+accountColumns.Status, in.Status)
	}
	kw := strings.TrimSpace(in.Keyword)
	if kw != "" {
		mod = mod.Where("(a.nickname LIKE ? OR a.username LIKE ?)", "%"+kw+"%", "%"+kw+"%")
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取账号总数失败")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("a." + accountColumns.Id).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取账号列表失败")
	}
	return
}

func (s *sSysPublish) taskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error) {
	taskColumns := pdao.YoubanPublishTask.Columns()
	mod := s.taskModel(ctx)
	if in.MerchantId > 0 {
		mod = mod.Where("t."+taskColumns.MerchantId, in.MerchantId)
	}
	if in.AccountId > 0 {
		mod = mod.Where("t."+taskColumns.AccountId, in.AccountId)
	}
	if in.Status != "" {
		mod = mod.Where("t."+taskColumns.Status, in.Status)
	}
	kw := strings.TrimSpace(in.Keyword)
	if kw != "" {
		mod = mod.Where("(t.title LIKE ? OR t.client_request_id LIKE ?)", "%"+kw+"%", "%"+kw+"%")
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取任务总数失败")
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("t." + taskColumns.Id).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取任务列表失败")
	}
	return
}

func (s *sSysPublish) taskModel(ctx context.Context) *gdb.Model {
	taskColumns := pdao.YoubanPublishTask.Columns()
	return pdao.YoubanPublishTask.DB().Model(pdao.YoubanPublishTask.Table()+" t").Safe().Ctx(ctx).
		LeftJoin(publishMerchantTable+" m", "m.id=t.merchant_id").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id").
		WhereNull("t." + taskColumns.DeletedAt).
		Fields("t.*,m.name AS merchant_name,a.nickname AS account_nickname,a.username AS account_username")
}

func (s *sSysPublish) saveTask(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	if err = in.Filter(ctx); err != nil {
		return 0, err
	}
	if in.MerchantId <= 0 {
		return 0, gerror.New("商家ID不能为空")
	}
	if in.AccountId <= 0 {
		return 0, gerror.New("上架账号ID不能为空")
	}
	if err = s.ensureAccountBelongsMerchant(ctx, in.AccountId, in.MerchantId); err != nil {
		return 0, err
	}
	taskColumns := pdao.YoubanPublishTask.Columns()
	if in.Id == 0 && strings.TrimSpace(in.ClientRequestId) != "" {
		existing, findErr := pdao.YoubanPublishTask.Ctx(ctx).
			Where(taskColumns.MerchantId, in.MerchantId).
			Where(taskColumns.ClientRequestId, strings.TrimSpace(in.ClientRequestId)).
			WhereNull(taskColumns.DeletedAt).
			Fields(taskColumns.Id).
			Value()
		if findErr != nil {
			return 0, gerror.Wrap(findErr, "检查幂等请求失败")
		}
		if existing.Int64() > 0 {
			return existing.Int64(), nil
		}
	}
	data := g.Map{
		taskColumns.MerchantId:      in.MerchantId,
		taskColumns.AccountId:       in.AccountId,
		taskColumns.ClientRequestId: strings.TrimSpace(in.ClientRequestId),
		taskColumns.Title:           strings.TrimSpace(in.Title),
		taskColumns.Province:        strings.TrimSpace(in.Province),
		taskColumns.City:            strings.TrimSpace(in.City),
		taskColumns.PlainText:       strings.TrimSpace(in.PlainText),
		taskColumns.TgPushEnabled:   in.TgPushEnabled,
		taskColumns.UpdatedBy:       contexts.GetUserId(ctx),
		taskColumns.UpdatedAt:       gtime.Now(),
	}
	if in.Id > 0 {
		_, err = pdao.YoubanPublishTask.Ctx(ctx).Where(taskColumns.Id, in.Id).WhereNull(taskColumns.DeletedAt).Data(data).Update()
		id = in.Id
	} else {
		data[taskColumns.Status] = sysin.PublishTaskStatusDraft
		data[taskColumns.TgStatus] = "pending"
		data[taskColumns.MediaCount] = 0
		data[taskColumns.CreatedBy] = contexts.GetUserId(ctx)
		data[taskColumns.CreatedAt] = gtime.Now()
		id, err = pdao.YoubanPublishTask.Ctx(ctx).Data(data).InsertAndGetId()
	}
	if err != nil {
		return 0, gerror.Wrap(err, "保存上架任务失败")
	}
	return
}

func (s *sSysPublish) submitTask(ctx context.Context, id int64, accountId int64) (err error) {
	task, err := s.getTask(ctx, id, accountId)
	if err != nil {
		return err
	}
	if task["status"].String() == sysin.PublishTaskStatusCanceled {
		return gerror.New("已取消的任务不能提交")
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
		"status":        sysin.PublishTaskStatusPending,
		"tg_status":     "pending",
		"error_message": "",
		"submitted_at":  gtime.Now(),
		"updated_by":    contexts.GetUserId(ctx),
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "提交上架任务失败")
	}
	task, err = s.getTask(ctx, id, accountId)
	if err != nil {
		return err
	}
	if _, err = s.publishTaskToProfile(ctx, task); err != nil {
		_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
			"status":        sysin.PublishTaskStatusFailed,
			"error_message": err.Error(),
			"updated_by":    contexts.GetUserId(ctx),
			"updated_at":    gtime.Now(),
		}).Update()
		return err
	}
	return s.ensureTgJob(ctx, id)
}

func (s *sSysPublish) cancelTask(ctx context.Context, id int64, accountId int64) (err error) {
	if _, err = s.getTask(ctx, id, accountId); err != nil {
		return err
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
		"status":     sysin.PublishTaskStatusCanceled,
		"updated_by": contexts.GetUserId(ctx),
		"updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "取消上架任务失败")
	}
	return nil
}

func (s *sSysPublish) ensureTgJob(ctx context.Context, taskId int64) error {
	row, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取上架任务失败")
	}
	if row.IsEmpty() || row["tg_push_enabled"].Int() != 1 {
		return nil
	}
	exists, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("task_id", taskId).Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG任务失败")
	}
	if exists > 0 {
		return nil
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"task_id":     taskId,
		"merchant_id": row["merchant_id"].Int64(),
		"account_id":  row["account_id"].Int64(),
		"profile_id":  row["profile_id"].Int64(),
		"status":      "pending",
		"created_at":  gtime.Now(),
		"updated_at":  gtime.Now(),
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "创建TG发布任务失败")
	}
	return nil
}

func (s *sSysPublish) getTask(ctx context.Context, id int64, accountId int64) (gdb.Record, error) {
	if id <= 0 {
		return nil, gerror.New("任务ID不能为空")
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", id).WhereNull("deleted_at")
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("上架任务不存在")
	}
	return row, nil
}

func (s *sSysPublish) currentAccount(ctx context.Context) (*sysin.AccountModel, error) {
	userId := contexts.GetUserId(ctx)
	if userId <= 0 {
		return nil, gerror.New("请先登录")
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	merchantColumns := pdao.YoubanPublishMerchant.Columns()
	var account *sysin.AccountModel
	err := pdao.YoubanPublishAccount.DB().Model(pdao.YoubanPublishAccount.Table()+" a").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishMerchant.Table()+" m", "m.id=a.merchant_id").
		Where("a."+accountColumns.AdminMemberId, userId).
		Where("a."+accountColumns.Status, 1).
		WhereNull("a." + accountColumns.DeletedAt).
		WhereNull("m." + merchantColumns.DeletedAt).
		Fields("a.*,m.name AS merchant_name").
		OrderAsc("a." + accountColumns.Id).
		Scan(&account)
	if err != nil {
		return nil, gerror.Wrap(err, "读取当前上架账号失败")
	}
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("当前用户未绑定上架账号")
	}
	return account, nil
}

func (s *sSysPublish) ensureMerchant(ctx context.Context, merchantId int64) error {
	merchantColumns := pdao.YoubanPublishMerchant.Columns()
	count, err := pdao.YoubanPublishMerchant.Ctx(ctx).Where(merchantColumns.Id, merchantId).WhereNull(merchantColumns.DeletedAt).Count()
	if err != nil {
		return gerror.Wrap(err, "检查商家失败")
	}
	if count == 0 {
		return gerror.New("商家不存在")
	}
	return nil
}

func (s *sSysPublish) ensureAccountBelongsMerchant(ctx context.Context, accountId int64, merchantId int64) error {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	count, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, accountId).
		Where(accountColumns.MerchantId, merchantId).
		WhereNull(accountColumns.DeletedAt).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查上架账号失败")
	}
	if count == 0 {
		return gerror.New("上架账号不存在或不属于该商家")
	}
	return nil
}
