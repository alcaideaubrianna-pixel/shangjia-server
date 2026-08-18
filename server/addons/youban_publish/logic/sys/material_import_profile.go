package sys

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/utility/file"
)

func (s *sSysPublish) saveMaterialImportGroupProfile(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel, items []collectMediaItem) (int64, error) {
	if task == nil || group == nil {
		return 0, gerror.New("资料导入分组不存在")
	}
	title := strings.TrimSpace(group.Title)
	if title == "" {
		title = fmt.Sprintf("%dTG%d", task.AccountId, group.Id)
	}
	channelIds := append([]int64(nil), task.ChannelIds...)
	input := &sysin.ProfileSaveInp{
		Id:         group.ProfileId,
		ChannelIds: channelIds,
		Title:      title,
		PlainText:  strings.TrimSpace(firstNonEmpty(group.ProfileText, group.RawText)),
		Visibility: consts.ContentVisibilityPublic,
		Status:     1,
	}
	metadata, err := s.enrichProfileMetadata(ctx, input.PlainText)
	if err != nil {
		return 0, err
	}
	input.Province = metadata.Province
	input.City = metadata.City
	input.Tag = metadata.Tag
	if group.ProfileId <= 0 {
		profileId, err := s.materialImportExistingProfile(ctx, group)
		if err != nil {
			return 0, err
		}
		input.Id = profileId
	}
	saved, err := s.saveProfile(ctx, input, task.TenantId, task.AccountId)
	if err != nil {
		return 0, err
	}
	if err = s.bindMaterialImportGroupProfile(ctx, group, saved); err != nil {
		return 0, err
	}
	if err = s.updateMaterialImportProfileSource(ctx, saved.Id, group, task, title); err != nil {
		return 0, err
	}
	media, err := s.saveMaterialImportProfileMedia(ctx, task, group, saved.Id, items)
	if err != nil {
		return 0, err
	}
	input.Id = saved.Id
	input.Media = media
	saved, err = s.saveProfile(ctx, input, task.TenantId, task.AccountId)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(group.Title) == "" {
		title = fmt.Sprintf("%d%s", task.AccountId, saved.ProfileNo)
		group.Title = title
	}
	if err = s.updateMaterialImportProfileSource(ctx, saved.Id, group, task, title); err != nil {
		return 0, err
	}
	if err = s.ensureMaterialImportTelegramIndex(ctx, task, group, saved.Id); err != nil {
		return 0, err
	}
	if err = s.syncProfileNoteIndex(ctx, saved.Id); err != nil {
		return 0, err
	}
	_ = s.appendMaterialImportPublishLog(ctx, task, saved.Id, "imported", fmt.Sprintf("资料导入完成：%s", strings.TrimSpace(title)))
	return saved.Id, nil
}

func materialImportRegionCodes(ctx context.Context, text string) (string, string, error) {
	index, err := getLegacyCMSRegionIndex(ctx)
	if err != nil {
		return "", "", err
	}
	province, city := materialImportRegionCodesFromIndex(text, index)
	return province, city, nil
}

func materialImportRegionCodesFromIndex(text string, index *legacyCMSRegionIndex) (string, string) {
	if index == nil {
		return "", ""
	}
	fields := materialImportRegionFieldsFromText(text)
	provinceIDs := make(map[int64]struct{})
	cityIDs := make(map[int64]struct{})
	for _, provinceText := range fields.provinceTexts {
		normalizedText := normalizeMaterialImportMatchText(provinceText)
		for name, province := range index.provincesByName {
			if province != nil && materialImportRegionNameMatches(normalizedText, name) {
				provinceIDs[province.Id] = struct{}{}
			}
		}
	}
	allowedProvinceIDs := make(map[int64]struct{}, len(provinceIDs))
	for provinceID := range provinceIDs {
		allowedProvinceIDs[provinceID] = struct{}{}
	}
	for _, cityText := range fields.cityTexts {
		materialImportMatchCityText(cityText, index, allowedProvinceIDs, provinceIDs, cityIDs)
	}
	if len(provinceIDs) == 0 {
		for _, provinceText := range fields.provinceTexts {
			materialImportMatchCityText(provinceText, index, nil, provinceIDs, cityIDs)
		}
	}
	for provinceID := range provinceIDs {
		if !materialImportMunicipalityProvince(provinceID) {
			continue
		}
		for _, city := range index.childrenByPid[provinceID] {
			if city != nil && city.Level == 2 {
				cityIDs[city.Id] = struct{}{}
				break
			}
		}
	}
	return joinMaterialImportRegionIDs(provinceIDs), joinMaterialImportRegionIDs(cityIDs)
}

type materialImportRegionFields struct {
	provinceTexts []string
	cityTexts     []string
}

var materialImportProvinceLabels = []string{
	"所在省份", "现居省份", "常住省份", "常驻省份", "省份", "所在省", "现居省",
}

var materialImportCityLabels = []string{
	"所在城市", "现居城市", "常住城市", "常驻城市", "城市", "所在地区", "现居地区", "地区", "所在地", "现居地", "位置", "所在市", "现居市",
}

func materialImportRegionFieldsFromText(text string) materialImportRegionFields {
	fields := materialImportRegionFields{}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\u00a0", " "))
		if line == "" {
			continue
		}
		if value, ok := materialImportRegionFieldValue(line, materialImportProvinceLabels); ok {
			fields.provinceTexts = append(fields.provinceTexts, value)
			continue
		}
		if value, ok := materialImportRegionFieldValue(line, materialImportCityLabels); ok {
			fields.cityTexts = append(fields.cityTexts, value)
		}
	}
	return fields
}

func materialImportRegionFieldValue(line string, labels []string) (string, bool) {
	normalizedLine := normalizeMaterialImportMatchText(line)
	for _, label := range labels {
		normalizedLabel := normalizeMaterialImportMatchText(label)
		if !strings.HasPrefix(normalizedLine, normalizedLabel) {
			continue
		}
		remainder := strings.TrimSpace(string([]rune(line)[len([]rune(label)):]))
		remainder = strings.TrimSpace(strings.TrimLeft(remainder, ":：;；,，-—"))
		if remainder != "" {
			return remainder, true
		}
	}
	return "", false
}

func materialImportMatchCityText(text string, index *legacyCMSRegionIndex, allowedProvinceIDs map[int64]struct{}, provinceIDs map[int64]struct{}, cityIDs map[int64]struct{}) {
	normalizedText := normalizeMaterialImportMatchText(text)
	for name, province := range index.provincesByName {
		if province == nil || !materialImportMunicipalityProvince(province.Id) || !materialImportRegionNameMatches(normalizedText, name) {
			continue
		}
		if len(allowedProvinceIDs) > 0 {
			if _, ok := allowedProvinceIDs[province.Id]; !ok {
				continue
			}
		}
		provinceIDs[province.Id] = struct{}{}
	}
	for name, cities := range index.citiesByName {
		for _, city := range cities {
			if city == nil || !materialImportRegionOptionMatches(normalizedText, name, city) || !materialImportRegionParentAllowed(city.Pid, allowedProvinceIDs) {
				continue
			}
			cityIDs[city.Id] = struct{}{}
			if city.Pid > 0 {
				provinceIDs[city.Pid] = struct{}{}
			}
		}
	}
	for name, districts := range index.districtsByName {
		for _, district := range districts {
			if district == nil || district.Pid <= 0 || !materialImportRegionOptionMatches(normalizedText, name, district) || materialImportRegionNameShadowed(normalizedText, name, index.districtsByName) || materialImportDistrictAliasConflictsWithCity(normalizedText, name, district, index) {
				continue
			}
			city := index.optionsById[district.Pid]
			if city == nil || city.Level != 2 || !materialImportRegionParentAllowed(city.Pid, allowedProvinceIDs) {
				continue
			}
			cityIDs[city.Id] = struct{}{}
			if city.Pid > 0 {
				provinceIDs[city.Pid] = struct{}{}
			}
		}
	}
}

func materialImportDistrictAliasConflictsWithCity(normalizedText string, name string, district *legacyCMSRegionOption, index *legacyCMSRegionIndex) bool {
	if district == nil || index == nil || materialImportRegionNameMatches(normalizedText, name) {
		return false
	}
	for _, alias := range materialImportRegionTitleAliases(district.Title) {
		if !materialImportRegionNameMatches(normalizedText, alias) {
			continue
		}
		normalizedAlias := normalizeMaterialImportMatchText(alias)
		for cityName := range index.citiesByName {
			if normalizeMaterialImportMatchText(cityName) == normalizedAlias {
				return true
			}
		}
	}
	return false
}

func materialImportRegionOptionMatches(normalizedText string, name string, option *legacyCMSRegionOption) bool {
	if materialImportRegionNameMatches(normalizedText, name) {
		return true
	}
	if option == nil {
		return false
	}
	for _, alias := range materialImportRegionTitleAliases(option.Title) {
		if materialImportRegionNameMatches(normalizedText, alias) {
			return true
		}
	}
	return false
}

func materialImportRegionTitleAliases(title string) []string {
	normalizedTitle := normalizeMaterialImportMatchText(title)
	aliases := []string{normalizedTitle}
	trimmed := strings.TrimSuffix(normalizedTitle, "自治州")
	trimmed = strings.TrimRight(trimmed, "省市县区旗盟州")
	if trimmed != "" && trimmed != normalizedTitle {
		aliases = append(aliases, trimmed)
	}
	ethnicMarkers := []string{"布依族", "苗族", "土家族", "傣族", "景颇族", "藏族", "羌族", "彝族", "哈尼族", "壮族", "侗族", "白族", "傈僳族", "蒙古族", "朝鲜族", "回族", "哈萨克", "柯尔克孜", "维吾尔"}
	markerIndex := -1
	for _, marker := range ethnicMarkers {
		if index := strings.Index(normalizedTitle, marker); index > 0 && (markerIndex < 0 || index < markerIndex) {
			markerIndex = index
		}
	}
	if markerIndex > 0 {
		aliases = append(aliases, normalizedTitle[:markerIndex])
	}
	return aliases
}

func materialImportRegionNameShadowed(normalizedText string, name string, candidates map[string][]*legacyCMSRegionOption) bool {
	normalizedName := normalizeMaterialImportMatchText(name)
	for candidate := range candidates {
		normalizedCandidate := normalizeMaterialImportMatchText(candidate)
		if len([]rune(normalizedCandidate)) <= len([]rune(normalizedName)) || !strings.Contains(normalizedCandidate, normalizedName) {
			continue
		}
		if materialImportRegionNameMatches(normalizedText, candidate) {
			return true
		}
	}
	return false
}

func materialImportRegionParentAllowed(provinceID int64, provinceIDs map[int64]struct{}) bool {
	if len(provinceIDs) == 0 {
		return true
	}
	_, ok := provinceIDs[provinceID]
	return ok
}

func materialImportMunicipalityProvince(provinceID int64) bool {
	switch provinceID {
	case 110000, 120000, 310000, 500000:
		return true
	default:
		return false
	}
}

func materialImportRegionNameMatches(normalizedText string, name string) bool {
	normalizedName := normalizeMaterialImportMatchText(name)
	return len([]rune(normalizedName)) >= 2 && strings.Contains(normalizedText, normalizedName)
}

func (s *sSysPublish) materialImportMatchedTags(ctx context.Context, text string) (string, error) {
	names, err := s.materialImportTagNames(ctx)
	if err != nil {
		return "", err
	}
	return materialImportMatchedTagNames(text, names), nil
}

func (s *sSysPublish) refreshMaterialImportProfileMetadata(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel, profileId int64) error {
	if task == nil || group == nil || profileId <= 0 {
		return nil
	}
	plainText := strings.TrimSpace(firstNonEmpty(group.ProfileText, group.RawText))
	metadata, err := s.enrichProfileMetadata(ctx, plainText)
	if err != nil {
		return err
	}
	columns := dao.ContentProfile.Columns()
	if _, err = dao.ContentProfile.Ctx(ctx).
		Where(columns.Id, profileId).
		WhereNull(columns.DeletedAt).
		Data(g.Map{
			columns.Province:        metadata.Province,
			columns.City:            metadata.City,
			columns.CupSize:         metadata.Tag,
			columns.SourceUpdateBy:  task.SourceTitle,
			columns.SourceUpdatedAt: gtime.Now(),
			columns.UpdatedAt:       gtime.Now(),
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "刷新TG导入资料地区标签失败")
	}
	return s.syncProfileNoteIndex(ctx, profileId)
}

func materialImportMatchedTagNames(text string, names []string) string {
	names = sortMaterialImportTagNames(names)
	normalizedText := normalizeMaterialImportMatchText(text)
	matched := make([]string, 0, len(names))
	for _, name := range names {
		normalizedName := normalizeMaterialImportMatchText(name)
		if normalizedName == "" || !strings.Contains(normalizedText, normalizedName) {
			continue
		}
		matched = append(matched, name)
	}
	return strings.Join(matched, ",")
}

func joinMaterialImportRegionIDs(ids map[int64]struct{}) string {
	values := make([]int64, 0, len(ids))
	for id := range ids {
		if id > 0 {
			values = append(values, id)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	parts := make([]string, 0, len(values))
	for _, id := range values {
		part := fmt.Sprintf("%d", id)
		if len(strings.Join(append(parts, part), ",")) > 64 {
			break
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ",")
}

func normalizeMaterialImportMatchText(value string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "", "\u00a0", "", "　", "", "，", ",").Replace(strings.TrimSpace(value)))
}

func sortMaterialImportTagNames(names []string) []string {
	result := append([]string(nil), names...)
	sort.SliceStable(result, func(i, j int) bool {
		return len([]rune(result[i])) > len([]rune(result[j]))
	})
	return result
}

func (s *sSysPublish) materialImportExistingProfile(ctx context.Context, group *sysin.MaterialImportGroupModel) (int64, error) {
	sourceKey := materialImportProfileSourceKey(group)
	profileCols := dao.ContentProfile.Columns()
	profile, err := dao.ContentProfile.Ctx(ctx).
		Fields(profileCols.Id).
		Where(profileCols.SourceKey, sourceKey).
		WhereNull(profileCols.DeletedAt).
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG导入资料失败")
	}
	if profile.IsEmpty() {
		return 0, nil
	}
	return profile[profileCols.Id].Int64(), nil
}

func (s *sSysPublish) bindMaterialImportGroupProfile(ctx context.Context, group *sysin.MaterialImportGroupModel, saved *sysin.ProfileSaveModel) error {
	if group == nil || group.Id <= 0 || saved == nil || saved.Id <= 0 {
		return nil
	}
	_, err := g.DB().Model(pdao.YoubanPublishMaterialImportGroup.Table()).Safe().Ctx(ctx).
		Where("id", group.Id).
		Data(g.Map{"profile_id": saved.Id, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "记录TG导入资料关联失败")
	}
	group.ProfileId = saved.Id
	return nil
}

func (s *sSysPublish) updateMaterialImportProfileSource(ctx context.Context, profileId int64, group *sysin.MaterialImportGroupModel, task *sysin.MaterialImportTaskModel, title string) error {
	profileCols := dao.ContentProfile.Columns()
	messageAt := materialImportMessageTime(group)
	_, err := dao.ContentProfile.Ctx(ctx).Where(profileCols.Id, profileId).Data(g.Map{
		profileCols.Title:           title,
		profileCols.SourceType:      publishProfileSourceType,
		profileCols.SourceKey:       materialImportProfileSourceKey(group),
		profileCols.ImportStatus:    "imported",
		profileCols.SourceCreateBy:  task.SourceTitle,
		profileCols.SourceUpdateBy:  task.SourceTitle,
		profileCols.SourceCreatedAt: messageAt,
		profileCols.SourceUpdatedAt: messageAt,
		profileCols.CreatedAt:       messageAt,
		profileCols.UpdatedAt:       messageAt,
	}).Update()
	return gerror.Wrap(err, "更新TG导入资料来源失败")
}

func (s *sSysPublish) saveMaterialImportProfileMedia(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel, profileId int64, items []collectMediaItem) ([]*sysin.ProfileMediaSaveItem, error) {
	ctx = importRuntimeContext(ctx, firstPositiveInt64(task.UpdatedBy, task.AccountId))
	owner, err := s.resolveMediaEditTask(ctx, &sysin.MediaUploadInp{ProfileId: profileId}, task.TenantId, task.AccountId)
	if err != nil {
		return nil, err
	}
	result := make([]*sysin.ProfileMediaSaveItem, 0, len(items))
	for index, item := range items {
		item = normalizeCollectMediaItem(item)
		mediaType := collectPublishMediaType(item.Type)
		path := strings.TrimSpace(item.StoragePath)
		if mediaType == "" || path == "" {
			continue
		}
		purpose := materialImportMediaPurpose(item)
		fileURL := normalizeMediaFileURL(item.FileUrl, path)
		localPath, cleanup, err := cachedTelegramMediaFile(ctx, &telegramMediaItem{MediaType: mediaType, FileUrl: fileURL, StoragePath: path})
		if err != nil {
			return nil, gerror.Wrap(err, "读取TG导入媒体失败")
		}
		if cleanup != nil {
			defer cleanup()
		}
		upload, err := materialImportUploadFileFromPath(localPath, fmt.Sprintf("material-import-%d-%d%s", group.Id, index+1, filepath.Ext(path)))
		if err != nil {
			return nil, err
		}
		media, uploadErr := s.saveUploadedTaskMedia(ctx, owner, &sysin.MediaUploadInp{
			ProfileId: profileId,
			MediaType: mediaType,
			Purpose:   purpose,
			SortIndex: index + 1,
		}, upload, nil, nil)
		if uploadErr != nil {
			return nil, uploadErr
		}
		result = append(result, &sysin.ProfileMediaSaveItem{MediaId: media.Id, Purpose: purpose, SortIndex: index + 1})
	}
	return result, nil
}

func (s *sSysPublish) saveMaterialImportProfileMissingMedia(ctx context.Context, task *sysin.MaterialImportTaskModel, group *sysin.MaterialImportGroupModel, profileId int64, items []collectMediaItem) error {
	if profileId <= 0 {
		return gerror.New("资料不存在，无法补齐TG媒体")
	}
	if _, err := s.saveMaterialImportProfileMedia(ctx, task, group, profileId, items); err != nil {
		return gerror.Wrap(err, "补齐TG导入资料媒体失败")
	}
	return nil
}

func materialImportMediaPurpose(item collectMediaItem) string {
	purpose := strings.TrimSpace(item.Purpose)
	if purpose == "verify" {
		return purpose
	}
	return "display"
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 1
}

func materialImportUploadFileFromPath(path string, name string) (*ghttp.UploadFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG导入媒体失败")
	}
	if len(content) == 0 {
		return nil, gerror.New("TG导入媒体为空")
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(path)
	}
	header, err := file.NewMultipartFileHeader(name, content)
	if err != nil {
		return nil, gerror.Wrap(err, "创建TG导入媒体上传文件失败")
	}
	return &ghttp.UploadFile{FileHeader: header}, nil
}

func materialImportProfileSourceKey(group *sysin.MaterialImportGroupModel) string {
	return "youban_publish:material_import:" + materialImportStableGroupKey(group)
}

func materialImportMessageTime(group *sysin.MaterialImportGroupModel) *gtime.Time {
	if group != nil && group.MessageAt != nil {
		return group.MessageAt
	}
	return gtime.Now()
}

func materialImportStableGroupKey(group *sysin.MaterialImportGroupModel) string {
	key := strings.TrimSpace(group.SourceUniqueKey)
	if key == "" {
		return ""
	}
	taskPrefix := fmt.Sprintf("task:%d:", group.TaskId)
	return strings.TrimPrefix(key, taskPrefix)
}

func (s *sSysPublish) appendMaterialImportPublishLog(ctx context.Context, task *sysin.MaterialImportTaskModel, profileId int64, status string, message string) error {
	if task == nil || profileId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":     0,
		"task_id":    task.Id,
		"tenant_id":  task.TenantId,
		"account_id": task.AccountId,
		"profile_id": profileId,
		"bot_id":     0,
		"action":     "material_import",
		"status":     status,
		"message":    strings.TrimSpace(message),
		"created_at": gtime.Now(),
	}).Insert()
	return err
}
