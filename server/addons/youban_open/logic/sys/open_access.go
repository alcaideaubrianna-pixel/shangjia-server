package sys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pdao "hotgo/addons/youban_open/internal/dao"
	"hotgo/addons/youban_open/model/input/sysin"
	"hotgo/addons/youban_open/service"
)

type sOpenAccess struct{}

func init() { service.RegisterOpenAccess(&sOpenAccess{}) }

func (s *sOpenAccess) AppList(ctx context.Context, in *sysin.CmsAppListInp) ([]*sysin.CmsAppModel, error) {
	columns := pdao.CmsApp.Columns()
	model := pdao.CmsApp.Ctx(ctx).Fields(columns.Id, columns.AppId, columns.Name, columns.BaseUrl, columns.InstanceId, columns.SourceIp, columns.CmsVersion, columns.LastHeartbeatAt, columns.Status, columns.CreatedAt, columns.UpdatedAt)
	if in != nil && strings.TrimSpace(in.Name) != "" {
		model = model.WhereLike(columns.Name, "%"+strings.TrimSpace(in.Name)+"%")
	}
	if in != nil && in.Status > 0 {
		model = model.Where(columns.Status, in.Status)
	}
	var list []*sysin.CmsAppModel
	if err := model.OrderDesc(columns.Id).Scan(&list); err != nil {
		return nil, gerror.Wrap(err, "读取CMS应用失败")
	}
	if list == nil {
		list = []*sysin.CmsAppModel{}
	}
	return list, nil
}

func (s *sOpenAccess) AppSave(ctx context.Context, in *sysin.CmsAppSaveInp) (*sysin.CmsAppCredentialModel, error) {
	columns := pdao.CmsApp.Columns()
	now := gtime.Now()
	if in.Id > 0 {
		result, err := pdao.CmsApp.Ctx(ctx).Where(columns.Id, in.Id).Data(g.Map{
			columns.Name: strings.TrimSpace(in.Name), columns.BaseUrl: strings.TrimSpace(in.BaseUrl),
			columns.Status: in.Status, columns.UpdatedAt: now,
		}).Update()
		if err != nil {
			return nil, gerror.Wrap(err, "更新CMS应用失败")
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return nil, gerror.New("CMS应用不存在")
		}
		return s.appCredential(ctx, in.Id, "")
	}
	appId, secret, err := newCmsCredential()
	if err != nil {
		return nil, err
	}
	result, err := pdao.CmsApp.Ctx(ctx).Data(g.Map{
		columns.AppId: appId, columns.AppSecret: secret, columns.Name: strings.TrimSpace(in.Name),
		columns.BaseUrl: strings.TrimSpace(in.BaseUrl), columns.ReviewMode: sysin.CmsReviewRequired, columns.Status: in.Status,
		columns.CreatedAt: now, columns.UpdatedAt: now,
	}).Insert()
	if err != nil {
		return nil, gerror.Wrap(err, "创建CMS应用失败")
	}
	id, _ := result.LastInsertId()
	return s.appCredential(ctx, id, secret)
}

func (s *sOpenAccess) AppResetSecret(ctx context.Context, in *sysin.CmsAppResetSecretInp) (*sysin.CmsAppCredentialModel, error) {
	_, secret, err := newCmsCredential()
	if err != nil {
		return nil, err
	}
	columns := pdao.CmsApp.Columns()
	result, err := pdao.CmsApp.Ctx(ctx).Where(columns.Id, in.Id).Data(g.Map{columns.AppSecret: secret, columns.UpdatedAt: gtime.Now()}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "重置CMS应用密钥失败")
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil, gerror.New("CMS应用不存在")
	}
	return s.appCredential(ctx, in.Id, secret)
}

func (s *sOpenAccess) appCredential(ctx context.Context, id int64, revealedSecret string) (*sysin.CmsAppCredentialModel, error) {
	columns := pdao.CmsApp.Columns()
	var app *sysin.CmsAppModel
	if err := pdao.CmsApp.Ctx(ctx).Fields(columns.Id, columns.AppId, columns.Name, columns.BaseUrl, columns.Status, columns.CreatedAt, columns.UpdatedAt).Where(columns.Id, id).Scan(&app); err != nil {
		return nil, gerror.Wrap(err, "读取CMS应用失败")
	}
	if app == nil {
		return nil, gerror.New("CMS应用不存在")
	}
	return &sysin.CmsAppCredentialModel{CmsAppModel: app, AppSecret: revealedSecret}, nil
}

func newCmsCredential() (string, string, error) {
	appBytes, secretBytes := make([]byte, 9), make([]byte, 32)
	if _, err := rand.Read(appBytes); err != nil {
		return "", "", gerror.Wrap(err, "生成应用ID失败")
	}
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", gerror.Wrap(err, "生成应用密钥失败")
	}
	return "xc_" + base64.RawURLEncoding.EncodeToString(appBytes), base64.RawURLEncoding.EncodeToString(secretBytes), nil
}

func newEnrollmentToken() (string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", gerror.Wrap(err, "生成实例令牌失败")
	}
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func hashEnrollmentToken(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(digest[:])
}

func (s *sOpenAccess) RegisterInstance(ctx context.Context, in *sysin.CmsInstanceRegisterInp, sourceIP string) (*sysin.CmsInstanceRegisterModel, error) {
	columns := pdao.CmsApp.Columns()
	instanceID := strings.TrimSpace(in.InstanceId)
	var app *sysin.CmsAppModel
	if err := pdao.CmsApp.Ctx(ctx).Where(columns.InstanceId, instanceID).Scan(&app); err != nil {
		return nil, gerror.Wrap(err, "读取CMS实例失败")
	}
	if app != nil {
		return s.instanceState(ctx, app, in.EnrollToken, "", "")
	}
	appID, secret, err := newCmsCredential()
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(in.EnrollToken)
	if token == "" {
		token, err = newEnrollmentToken()
		if err != nil {
			return nil, err
		}
	}
	now := gtime.Now()
	insertResult, err := pdao.CmsApp.Ctx(ctx).Data(g.Map{
		columns.AppId: appID, columns.AppSecret: secret, columns.InstanceId: instanceID,
		columns.EnrollHash: hashEnrollmentToken(token), columns.Name: strings.TrimSpace(in.Name),
		columns.BaseUrl: strings.TrimSpace(in.BaseUrl), columns.SourceIp: strings.TrimSpace(sourceIP),
		columns.CmsVersion: strings.TrimSpace(in.Version), columns.ReviewMode: sysin.CmsReviewRequired, columns.Status: 1,
		columns.LastHeartbeatAt: now, columns.CreatedAt: now, columns.UpdatedAt: now,
	}).Insert()
	if err != nil {
		return nil, gerror.Wrap(err, "注册CMS实例失败")
	}
	id, _ := insertResult.LastInsertId()
	return s.instanceState(ctx, &sysin.CmsAppModel{
		Id: id, AppId: appID, AppSecret: secret, InstanceId: instanceID, Status: 1,
	}, token, hashEnrollmentToken(token), "")
}

func (s *sOpenAccess) HeartbeatInstance(ctx context.Context, in *sysin.CmsInstanceHeartbeatInp, sourceIP string) (*sysin.CmsInstanceRegisterModel, error) {
	columns := pdao.CmsApp.Columns()
	var app *sysin.CmsAppModel
	if err := pdao.CmsApp.Ctx(ctx).Where(columns.InstanceId, strings.TrimSpace(in.InstanceId)).Scan(&app); err != nil {
		return nil, gerror.Wrap(err, "读取CMS实例失败")
	}
	if app == nil {
		return nil, gerror.New("CMS实例尚未注册")
	}
	if hashEnrollmentToken(in.EnrollToken) != appEnrollHash(ctx, app.Id) {
		return nil, gerror.New("CMS实例令牌无效")
	}
	_, err := pdao.CmsApp.Ctx(ctx).Where(columns.Id, app.Id).Data(g.Map{
		columns.SourceIp: strings.TrimSpace(sourceIP), columns.BaseUrl: strings.TrimSpace(in.BaseUrl),
		columns.CmsVersion: strings.TrimSpace(in.Version), columns.LastHeartbeatAt: gtime.Now(), columns.UpdatedAt: gtime.Now(),
	}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "更新CMS实例心跳失败")
	}
	return s.instanceState(ctx, app, in.EnrollToken, appEnrollHash(ctx, app.Id), in.CredentialVersion)
}

func appEnrollHash(ctx context.Context, id int64) string {
	columns := pdao.CmsApp.Columns()
	value, _ := pdao.CmsApp.Ctx(ctx).Where(columns.Id, id).Value(columns.EnrollHash)
	return value.String()
}

func credentialVersion(appId, secret string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(appId) + "\x00" + secret))
	return hex.EncodeToString(digest[:8])
}

func (s *sOpenAccess) instanceState(ctx context.Context, app *sysin.CmsAppModel, token, knownHash, clientVersion string) (*sysin.CmsInstanceRegisterModel, error) {
	if knownHash == "" {
		knownHash = appEnrollHash(ctx, app.Id)
	}
	if token != "" && hashEnrollmentToken(token) != knownHash {
		return nil, gerror.New("CMS实例令牌无效")
	}
	status := "pending"
	if app.Status == 1 {
		status = "approved"
	} else if app.Status == 3 {
		status = "disabled"
	} else if app.Status == 4 {
		status = "revoked"
	}
	result := &sysin.CmsInstanceRegisterModel{InstanceId: app.InstanceId, AppName: app.Name, Status: status}
	if status == "approved" && token != "" {
		columns := pdao.CmsApp.Columns()
		var credential struct {
			AppId     string `json:"appId"`
			AppSecret string `json:"appSecret"`
		}
		if err := pdao.CmsApp.Ctx(ctx).Fields(columns.AppId, columns.AppSecret).Where(columns.Id, app.Id).Scan(&credential); err != nil {
			return nil, err
		}
		result.CredentialVersion = credentialVersion(credential.AppId, credential.AppSecret)
		result.CredentialChanged = strings.TrimSpace(clientVersion) != result.CredentialVersion
		if result.CredentialChanged {
			result.AppId, result.AppSecret = credential.AppId, credential.AppSecret
		}
	}
	return result, nil
}

func (s *sOpenAccess) AppSecret(ctx context.Context, appId string) (string, error) {
	appId = strings.TrimSpace(appId)
	if appId == "" {
		return "", gerror.New("开放接口凭证无效")
	}
	columns := pdao.CmsApp.Columns()
	var app *sysin.CmsAppModel
	err := pdao.CmsApp.Ctx(ctx).
		Where(columns.AppId, appId).
		Where(columns.Status, 1).
		Scan(&app)
	if err != nil {
		return "", gerror.Wrap(err, "读取开放应用失败")
	}
	if app == nil || app.AppSecret == "" {
		return s.migrateConfiguredApp(ctx, appId)
	}
	return app.AppSecret, nil
}

func (s *sOpenAccess) migrateConfiguredApp(ctx context.Context, appId string) (string, error) {
	configuredId := g.Cfg().MustGet(ctx, "youbanPublish.open.appId", "").String()
	secret := g.Cfg().MustGet(ctx, "youbanPublish.open.appSecret", "").String()
	if appId != configuredId || secret == "" {
		return "", gerror.New("开放接口凭证无效")
	}
	columns := pdao.CmsApp.Columns()
	_, err := pdao.CmsApp.Ctx(ctx).Data(g.Map{
		columns.AppId: appId, columns.AppSecret: secret, columns.Name: appId,
		columns.Status: 1, columns.CreatedAt: gtime.Now(), columns.UpdatedAt: gtime.Now(),
	}).Insert()
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		return "", gerror.Wrap(err, "初始化开放应用失败")
	}
	return secret, nil
}

func (s *sOpenAccess) AllowedTenantIds(ctx context.Context, appId string) ([]int64, error) {
	columns := pdao.CmsTenantBinding.Columns()
	ids, err := pdao.CmsTenantBinding.Ctx(ctx).
		Fields(columns.TenantId).
		Where(columns.AppId, appId).
		Where(columns.Status, sysin.CmsBindingApproved).
		Array(columns.TenantId)
	if err != nil {
		return nil, gerror.Wrap(err, "读取CMS绑定租户失败")
	}
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id.Int64() > 0 {
			result = append(result, id.Int64())
		}
	}
	return result, nil
}

func (s *sOpenAccess) SaveBindingCode(ctx context.Context, appId string, in *sysin.CmsBindingCodeSaveInp) (*sysin.CmsBindingCodeModel, error) {
	code := normalizeBindingCode(in.Code)
	if code == "" {
		return nil, gerror.New("请输入绑定码")
	}
	columns := pdao.CmsBindingCode.Columns()
	bindColumns := pdao.CmsTenantBinding.Columns()
	version := 1
	err := pdao.CmsBindingCode.Ctx(ctx).Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var current struct {
			Version  int    `json:"version"`
			CodeHash string `json:"codeHash"`
		}
		if err := tx.Model(pdao.CmsBindingCode.Table()).Where(columns.AppId, appId).Scan(&current); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current.Version > 0 {
			if current.CodeHash == hashBindingCode(code) {
				version = current.Version
				return nil
			}
			version = current.Version + 1
			_, err := tx.Model(pdao.CmsBindingCode.Table()).Where(columns.AppId, appId).Data(g.Map{
				columns.CodeHash: hashBindingCode(code), columns.CodeHint: bindingCodeHint(code),
				columns.Version: version, columns.Status: 1, columns.UpdatedAt: gtime.Now(),
			}).Update()
			if err != nil {
				return err
			}
		} else {
			_, err := tx.Model(pdao.CmsBindingCode.Table()).Data(g.Map{
				columns.AppId: appId, columns.CodeHash: hashBindingCode(code), columns.CodeHint: bindingCodeHint(code),
				columns.Version: version, columns.Status: 1, columns.CreatedAt: gtime.Now(), columns.UpdatedAt: gtime.Now(),
			}).Insert()
			if err != nil {
				return err
			}
		}
		_, err := tx.Model(pdao.CmsTenantBinding.Table()).
			Where(bindColumns.AppId, appId).
			Where(bindColumns.Status, sysin.CmsBindingPending).
			Data(g.Map{bindColumns.Status: sysin.CmsBindingRevoked, bindColumns.Reason: "绑定码已刷新", bindColumns.UpdatedAt: gtime.Now()}).Update()
		return err
	})
	if err != nil {
		return nil, gerror.Wrap(err, "保存绑定码失败")
	}
	return &sysin.CmsBindingCodeModel{Version: version, Hint: bindingCodeHint(code)}, nil
}

func (s *sOpenAccess) ClaimBinding(ctx context.Context, tenantId int64, in *sysin.CmsBindingClaimInp) (*sysin.CmsBindingModel, error) {
	if err := ensureBindingTables(ctx); err != nil {
		return nil, gerror.Wrap(err, "初始化CMS绑定表失败")
	}
	if tenantId <= 0 {
		return nil, gerror.New("租户身份无效")
	}
	codeColumns := pdao.CmsBindingCode.Columns()
	var codeRow struct {
		AppId   string `json:"appId"`
		Version int    `json:"version"`
	}
	err := pdao.CmsBindingCode.Ctx(ctx).
		Where(codeColumns.CodeHash, hashBindingCode(normalizeBindingCode(in.Code))).
		Where(codeColumns.Status, 1).
		Scan(&codeRow)
	if err != nil {
		return nil, gerror.Wrap(err, "校验绑定码失败")
	}
	if codeRow.AppId == "" {
		return nil, gerror.New("绑定码无效")
	}

	columns := pdao.CmsTenantBinding.Columns()
	var existing *sysin.CmsBindingModel
	if err = pdao.CmsTenantBinding.Ctx(ctx).Where(columns.AppId, codeRow.AppId).Where(columns.TenantId, tenantId).Scan(&existing); err != nil {
		return nil, gerror.Wrap(err, "读取绑定记录失败")
	}
	if existing != nil && existing.Status == sysin.CmsBindingBlocked {
		return nil, gerror.New("该CMS已禁止当前租户绑定")
	}
	if existing != nil && existing.Status == sysin.CmsBindingApproved {
		return existing, nil
	}
	appColumns := pdao.CmsApp.Columns()
	reviewMode, err := pdao.CmsApp.Ctx(ctx).
		Where(appColumns.AppId, codeRow.AppId).
		Where(appColumns.Status, 1).
		Value(appColumns.ReviewMode)
	if err != nil {
		return nil, gerror.Wrap(err, "读取CMS审核策略失败")
	}
	if reviewMode.IsEmpty() {
		return nil, gerror.New("CMS应用不可用")
	}
	status := sysin.CmsBindingPending
	if reviewMode.String() == sysin.CmsReviewAutomatic {
		status = sysin.CmsBindingApproved
	}
	now := gtime.Now()
	data := g.Map{columns.CodeVersion: codeRow.Version, columns.Status: status,
		columns.Reason: "", columns.RequestedAt: now, columns.UpdatedAt: now}
	if status == sysin.CmsBindingApproved {
		data[columns.ReviewedAt] = now
	}
	if existing != nil {
		_, err = pdao.CmsTenantBinding.Ctx(ctx).Where(columns.Id, existing.Id).Data(data).Update()
	} else {
		data[columns.AppId], data[columns.TenantId], data[columns.CreatedAt] = codeRow.AppId, tenantId, now
		_, err = pdao.CmsTenantBinding.Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return nil, gerror.Wrap(err, "申请CMS绑定失败")
	}
	binding, err := s.findBinding(ctx, codeRow.AppId, tenantId, 0)
	if err == nil && binding != nil && binding.Status == sysin.CmsBindingApproved {
		s.emitApproved(ctx, binding)
	}
	return binding, err
}

func (s *sOpenAccess) RevokeTenantBinding(ctx context.Context, tenantId int64, in *sysin.CmsBindingRevokeInp) (*sysin.CmsBindingModel, error) {
	columns := pdao.CmsTenantBinding.Columns()
	var current *sysin.CmsBindingModel
	if err := pdao.CmsTenantBinding.Ctx(ctx).Where(columns.Id, in.Id).Where(columns.TenantId, tenantId).Scan(&current); err != nil {
		return nil, gerror.Wrap(err, "读取绑定记录失败")
	}
	if current == nil {
		return nil, gerror.New("绑定记录不存在")
	}
	if current.Status == sysin.CmsBindingBlocked {
		return nil, gerror.New("已拉黑的绑定不能由租户解除")
	}
	_, err := pdao.CmsTenantBinding.Ctx(ctx).Where(columns.Id, in.Id).Where(columns.TenantId, tenantId).Data(g.Map{
		columns.Status: sysin.CmsBindingRevoked, columns.Reason: "租户主动解除", columns.UpdatedAt: gtime.Now(),
	}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "解除平台绑定失败")
	}
	return s.findBinding(ctx, current.AppId, tenantId, in.Id)
}

func (s *sOpenAccess) SaveAppSettings(ctx context.Context, appId string, in *sysin.CmsAppSettingsInp) (*sysin.CmsAppSettingsModel, error) {
	if err := ensureBindingTables(ctx); err != nil {
		return nil, gerror.Wrap(err, "初始化CMS配置失败")
	}
	columns := pdao.CmsApp.Columns()
	result, err := pdao.CmsApp.Ctx(ctx).Where(columns.AppId, appId).Data(g.Map{
		columns.ReviewMode: in.ReviewMode, columns.UpdatedAt: gtime.Now(),
	}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "保存平台配置失败")
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil, gerror.New("CMS应用不存在")
	}
	return &sysin.CmsAppSettingsModel{ReviewMode: in.ReviewMode}, nil
}

func (s *sOpenAccess) LookupBindingCode(ctx context.Context, code string) (*sysin.CmsBindingLookupModel, error) {
	codeColumns, appColumns := pdao.CmsBindingCode.Columns(), pdao.CmsApp.Columns()
	var result *sysin.CmsBindingLookupModel
	err := pdao.CmsBindingCode.Ctx(ctx).As("c").LeftJoin(pdao.CmsApp.Table()+" a", "a."+appColumns.AppId+"=c."+codeColumns.AppId).
		Fields("c."+codeColumns.AppId+" AS app_id", "a."+appColumns.Name+" AS app_name").
		Where("c."+codeColumns.CodeHash, hashBindingCode(normalizeBindingCode(code))).Where("c."+codeColumns.Status, 1).Scan(&result)
	if err != nil {
		return nil, gerror.Wrap(err, "查询平台绑定码失败")
	}
	if result == nil || result.AppId == "" {
		return nil, gerror.New("未找到对应平台")
	}
	return result, nil
}

func (s *sOpenAccess) TenantBinding(ctx context.Context, tenantId int64) ([]*sysin.CmsBindingModel, error) {
	columns := pdao.CmsTenantBinding.Columns()
	return s.bindingList(ctx, pdao.CmsTenantBinding.Ctx(ctx).As("b").Where("b."+columns.TenantId, tenantId))
}

func (s *sOpenAccess) Bindings(ctx context.Context, appId string, in *sysin.CmsBindingListInp) ([]*sysin.CmsBindingModel, error) {
	columns := pdao.CmsTenantBinding.Columns()
	model := pdao.CmsTenantBinding.Ctx(ctx).As("b").Where("b."+columns.AppId, appId)
	if in != nil && in.Status != "" {
		model = model.Where("b."+columns.Status, in.Status)
	}
	return s.bindingList(ctx, model)
}

func (s *sOpenAccess) AdminBindings(ctx context.Context, in *sysin.CmsBindingListInp) ([]*sysin.CmsBindingModel, error) {
	columns := pdao.CmsTenantBinding.Columns()
	model := pdao.CmsTenantBinding.Ctx(ctx).As("b")
	if in != nil && strings.TrimSpace(in.Status) != "" {
		model = model.Where("b."+columns.Status, strings.TrimSpace(in.Status))
	}
	return s.bindingList(ctx, model)
}

func normalizeBindingCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }
func hashBindingCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
func bindingCodeHint(code string) string {
	if len(code) <= 4 {
		return code
	}
	return code[:2] + "****" + code[len(code)-2:]
}
