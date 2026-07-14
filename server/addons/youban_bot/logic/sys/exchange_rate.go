package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/casbin/govaluate"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/internal/library/cache"
)

const (
	exchangeRateDefaultSource       = "binance"
	exchangeRateDefaultRows         = 10
	exchangeRateDefaultCacheMinutes = 30
	exchangeRateAsset               = "USDT"
	exchangeRateFiat                = "CNY"
)

var exchangeRateExpressionRegexp = regexp.MustCompile(`^[0-9+\-*/().\s]+$`)

type exchangeRateFeature struct{}

func (exchangeRateFeature) Key() string         { return "exchange_rate" }
func (exchangeRateFeature) Command() string     { return "rate" }
func (exchangeRateFeature) Description() string { return "实时汇率" }
func (exchangeRateFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "source", Label: "行情来源", Component: "select", Default: exchangeRateDefaultSource, Placeholder: "当前支持 Binance P2P", Options: []*sysin.FeatureConfigOption{{Label: "Binance P2P", Value: "binance"}}},
		{Field: "rows", Label: "展示条数", Component: "input", Default: exchangeRateDefaultRows, Placeholder: "默认展示 10 条"},
		{Field: "cacheMinutes", Label: "缓存分钟", Component: "input", Default: exchangeRateDefaultCacheMinutes, Placeholder: "默认 30 分钟"},
		{Field: "aliases", Label: "触发别名", Component: "input", Default: "实时汇率,汇率,U价,u,z0,lk", Placeholder: "多个别名用英文逗号分隔"},
	}
}
func (exchangeRateFeature) Match(ctx context.Context, bot *sSysBot, row *botFeatureRow, text string) bool {
	_, matched := bot.parseExchangeRateRequest(ctx, text, "")
	return matched
}
func (exchangeRateFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx == nil || featureCtx.Msg == nil {
		return true, nil
	}
	req, matched := bot.parseExchangeRateRequest(ctx, featureCtx.Text, featureCtx.Args)
	if !matched {
		return false, nil
	}
	if req.ExprErr != nil {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), html.EscapeString(req.ExprErr.Error()))
	}
	res, err := bot.exchangeRateQuote(ctx, req.PayType)
	if err != nil {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), html.EscapeString(err.Error()))
	}
	text := bot.formatExchangeRateReply(ctx, req, res)
	return true, bot.replyExchangeRate(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), text, req.PayType)
}

type exchangeRateRequest struct {
	PayType string
	Expr    string
	Amount  float64
	HasExpr bool
	ExprErr error
}

type exchangeRateQuote struct {
	Source    string                   `json:"source"`
	Asset     string                   `json:"asset"`
	Fiat      string                   `json:"fiat"`
	PayType   string                   `json:"payType"`
	FetchedAt string                   `json:"fetchedAt"`
	Items     []*exchangeRateQuoteItem `json:"items"`
}

type exchangeRateQuoteItem struct {
	Price      float64  `json:"price"`
	PriceText  string   `json:"priceText"`
	Merchant   string   `json:"merchant"`
	PayMethods []string `json:"payMethods"`
}

type binanceP2PResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    []struct {
		Adv struct {
			Price        string `json:"price"`
			TradeMethods []struct {
				PayType              string `json:"payType"`
				TradeMethodName      string `json:"tradeMethodName"`
				TradeMethodShortName string `json:"tradeMethodShortName"`
			} `json:"tradeMethods"`
		} `json:"adv"`
		Advertiser struct {
			NickName string `json:"nickName"`
		} `json:"advertiser"`
	} `json:"data"`
}

func (s *sSysBot) parseExchangeRateRequest(ctx context.Context, text string, args string) (*exchangeRateRequest, bool) {
	text = strings.TrimSpace(text)
	args = strings.TrimSpace(args)
	if text == "" {
		return nil, false
	}
	content := args
	if content == "" {
		command, commandArgs := botCommandAndArgs(text)
		if command != "" {
			if command != strings.ToLower(strings.TrimPrefix(exchangeRateFeature{}.Command(), "/")) {
				return nil, false
			}
			content = commandArgs
		} else {
			alias, rest, ok := s.matchExchangeRateAlias(ctx, text)
			if !ok {
				return nil, false
			}
			_ = alias
			content = rest
		}
	}
	req := &exchangeRateRequest{PayType: "all"}
	content = strings.TrimSpace(strings.TrimPrefix(content, ":"))
	content = strings.TrimSpace(strings.TrimPrefix(content, "："))
	parts := strings.Fields(content)
	remain := make([]string, 0, len(parts))
	for _, part := range parts {
		payType, ok := normalizeExchangeRatePayType(part)
		if ok {
			req.PayType = payType
			continue
		}
		remain = append(remain, part)
	}
	if len(parts) == 1 && len(remain) == 1 {
		if payType, ok := normalizeExchangeRatePayType(parts[0]); ok {
			req.PayType = payType
			remain = nil
		}
	}
	expr := strings.TrimSpace(strings.Join(remain, ""))
	if expr != "" {
		amount, err := evaluateExchangeRateExpression(expr)
		if err != nil {
			req.Expr = expr
			req.HasExpr = true
			req.ExprErr = err
			return req, true
		}
		req.Expr = expr
		req.Amount = amount
		req.HasExpr = true
	}
	return req, true
}

func (s *sSysBot) matchExchangeRateAlias(ctx context.Context, text string) (alias string, rest string, ok bool) {
	clean := strings.TrimSpace(text)
	if payType, matched := normalizeExchangeRatePayType(clean); matched {
		return clean, exchangeRatePayTypeLabel(payType), true
	}
	parts := strings.Fields(clean)
	if len(parts) > 1 {
		if _, matched := normalizeExchangeRatePayType(parts[0]); matched {
			return parts[0], strings.TrimSpace(strings.TrimPrefix(clean, parts[0])), true
		}
	}
	aliases := s.exchangeRateAliases(ctx)
	lower := strings.ToLower(clean)
	for _, item := range aliases {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		itemLower := strings.ToLower(item)
		if lower == itemLower {
			return item, "", true
		}
		if strings.HasPrefix(lower, itemLower+" ") || strings.HasPrefix(lower, itemLower+"：") || strings.HasPrefix(lower, itemLower+":") {
			return item, strings.TrimSpace(clean[len(item):]), true
		}
	}
	return "", "", false
}

func (s *sSysBot) exchangeRateAliases(ctx context.Context) []string {
	value := s.featureConfigValue(ctx, exchangeRateFeature{}.Key(), "aliases")
	if value == "" {
		value = "实时汇率,汇率,U价,u,z0,lk"
	}
	list := strings.Split(value, ",")
	res := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item != "" {
			res = append(res, item)
		}
	}
	return res
}

func normalizeExchangeRatePayType(value string) (string, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "✅"))
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "", "all", "全部", "所有":
		return "all", value != ""
	case "bank", "银行卡", "银行", "卡":
		return "bank", true
	case "alipay", "支付宝", "支":
		return "alipay", true
	case "wechat", "微信", "wx", "微":
		return "wechat", true
	default:
		return "", false
	}
}

func exchangeRatePayTypeLabel(payType string) string {
	switch payType {
	case "bank":
		return "银行卡"
	case "alipay":
		return "支付宝"
	case "wechat":
		return "微信"
	default:
		return "所有"
	}
}

func exchangeRateBinancePayTypes(payType string) []string {
	switch payType {
	case "bank":
		return []string{"BANK"}
	case "alipay":
		return []string{"ALIPAY"}
	case "wechat":
		return []string{"WECHAT"}
	default:
		return []string{}
	}
}

func evaluateExchangeRateExpression(expr string) (float64, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, nil
	}
	if len(expr) > 80 || !exchangeRateExpressionRegexp.MatchString(expr) {
		return 0, gerror.New("仅支持数字和加减乘除计算")
	}
	expression, err := govaluate.NewEvaluableExpression(expr)
	if err != nil {
		return 0, gerror.New("计算表达式格式不正确")
	}
	value, err := expression.Evaluate(nil)
	if err != nil {
		return 0, gerror.New("计算表达式执行失败")
	}
	amount, ok := toFloat64(value)
	if !ok || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, gerror.New("计算结果不合法")
	}
	if amount < 0 {
		return 0, gerror.New("计算金额不能小于0")
	}
	return amount, nil
}

func toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint64:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func (s *sSysBot) exchangeRateQuote(ctx context.Context, payType string) (*exchangeRateQuote, error) {
	payType, ok := normalizeExchangeRatePayType(payType)
	if !ok {
		payType = "all"
	}
	source := strings.ToLower(strings.TrimSpace(s.featureConfigValue(ctx, exchangeRateFeature{}.Key(), "source")))
	if source == "" {
		source = exchangeRateDefaultSource
	}
	if source != "binance" {
		return nil, gerror.New("当前仅支持 Binance P2P 行情源")
	}
	return s.exchangeRateQuoteWithCache(ctx, source, payType)
}

func (s *sSysBot) exchangeRateQuoteWithCache(ctx context.Context, source string, payType string) (*exchangeRateQuote, error) {
	cacheKey := fmt.Sprintf("youban_bot:exchange_rate:%s:%s:%s:%s", source, strings.ToLower(exchangeRateAsset), strings.ToLower(exchangeRateFiat), payType)
	if cacheVar, err := cache.Instance().Get(ctx, cacheKey); err == nil && !cacheVar.IsNil() {
		var item exchangeRateQuote
		if err = json.Unmarshal([]byte(cacheVar.String()), &item); err == nil && len(item.Items) > 0 {
			return &item, nil
		}
	}
	quote, err := s.fetchBinanceExchangeRate(ctx, payType)
	if err != nil {
		return nil, err
	}
	if quote != nil && len(quote.Items) > 0 {
		bs, _ := json.Marshal(quote)
		_ = cache.Instance().Set(ctx, cacheKey, string(bs), s.exchangeRateCacheTTL(ctx))
	}
	return quote, nil
}

func (s *sSysBot) exchangeRateCacheTTL(ctx context.Context) time.Duration {
	minutes := exchangeRateDefaultCacheMinutes
	value := strings.TrimSpace(s.featureConfigValue(ctx, exchangeRateFeature{}.Key(), "cacheMinutes"))
	if value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			minutes = n
		}
	}
	return time.Duration(minutes) * time.Minute
}

func (s *sSysBot) exchangeRateRows(ctx context.Context) int {
	rows := exchangeRateDefaultRows
	value := strings.TrimSpace(s.featureConfigValue(ctx, exchangeRateFeature{}.Key(), "rows"))
	if value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 && n <= 20 {
			rows = n
		}
	}
	return rows
}

func (s *sSysBot) fetchBinanceExchangeRate(ctx context.Context, payType string) (*exchangeRateQuote, error) {
	rows := s.exchangeRateRows(ctx)
	payload := g.Map{
		"asset":         exchangeRateAsset,
		"fiat":          exchangeRateFiat,
		"tradeType":     "BUY",
		"page":          1,
		"rows":          rows,
		"payTypes":      exchangeRateBinancePayTypes(payType),
		"publisherType": nil,
	}
	client := g.Client().SetTimeout(15 * time.Second).ContentJson().SetHeaderMap(map[string]string{
		"User-Agent": "Mozilla/5.0 (YoubanBot/1.0)",
		"Origin":     "https://p2p.binance.com",
	})
	resp, err := client.Post(ctx, "https://p2p.binance.com/bapi/c2c/v2/friendly/c2c/adv/search", payload)
	if err != nil {
		return nil, gerror.Wrap(err, "获取实时汇率失败")
	}
	defer resp.Close()
	body := resp.ReadAll()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, gerror.Newf("获取实时汇率失败，行情接口状态：%d", resp.StatusCode)
	}
	var apiRes binanceP2PResponse
	if err = json.Unmarshal(body, &apiRes); err != nil {
		return nil, gerror.Wrap(err, "解析实时汇率失败")
	}
	if apiRes.Code != "000000" {
		msg := strings.TrimSpace(apiRes.Message)
		if msg == "" {
			msg = "行情接口返回异常"
		}
		return nil, gerror.New(msg)
	}
	items := make([]*exchangeRateQuoteItem, 0, len(apiRes.Data))
	for _, row := range apiRes.Data {
		price, err := strconv.ParseFloat(strings.TrimSpace(row.Adv.Price), 64)
		if err != nil || price <= 0 {
			continue
		}
		methods := make([]string, 0, len(row.Adv.TradeMethods))
		for _, method := range row.Adv.TradeMethods {
			name := firstNonEmpty(method.TradeMethodShortName, method.TradeMethodName, method.PayType)
			if strings.TrimSpace(name) != "" {
				methods = append(methods, name)
			}
		}
		items = append(items, &exchangeRateQuoteItem{Price: price, PriceText: row.Adv.Price, Merchant: row.Advertiser.NickName, PayMethods: methods})
	}
	if len(items) == 0 {
		return nil, gerror.New("暂未获取到实时汇率")
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Price < items[j].Price })
	return &exchangeRateQuote{Source: "Binance", Asset: exchangeRateAsset, Fiat: exchangeRateFiat, PayType: payType, FetchedAt: gtime.Now().Format("Y-m-d H:i:s"), Items: items}, nil
}

func (s *sSysBot) formatExchangeRateReply(ctx context.Context, req *exchangeRateRequest, quote *exchangeRateQuote) string {
	if req == nil {
		req = &exchangeRateRequest{PayType: "all"}
	}
	if quote == nil || len(quote.Items) == 0 {
		return "暂未获取到实时汇率"
	}
	lowest := quote.Items[0].Price
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s · %s · %s/%s %s\n\n", exchangeRatePayTypeLabel(req.PayType), html.EscapeString(quote.Source), quote.Asset, quote.Fiat, quote.FetchedAt))
	limit := s.exchangeRateRows(ctx)
	if limit > len(quote.Items) {
		limit = len(quote.Items)
	}
	for i := 0; i < limit; i++ {
		item := quote.Items[i]
		merchant := html.EscapeString(firstNonEmpty(item.Merchant, "匿名商家"))
		b.WriteString(fmt.Sprintf("%d) %s  %s\n", i+1, html.EscapeString(item.PriceText), merchant))
	}
	amount := 0.0
	amountText := "0"
	if req.HasExpr {
		amount = req.Amount
		amountText = formatExchangeRateNumber(amount, 4)
	}
	uAmount := 0.0
	if lowest > 0 {
		uAmount = amount / lowest
	}
	b.WriteString(fmt.Sprintf("\n💰 %s / %s = %s U\n", html.EscapeString(amountText), html.EscapeString(quote.Items[0].PriceText), html.EscapeString(formatExchangeRateNumber(uAmount, 4))))
	b.WriteString("\n帮助：\n")
	b.WriteString("实时汇率 1000*0.95/5\n")
	b.WriteString("z0 支付宝 1000\n")
	b.WriteString("lk 微信 500")
	return b.String()
}

func formatExchangeRateNumber(value float64, precision int) string {
	if math.Abs(value-math.Round(value)) < 0.0000001 {
		return fmt.Sprintf("%.0f", value)
	}
	text := strconv.FormatFloat(value, 'f', precision, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	return text
}

func (s *sSysBot) replyExchangeRate(ctx context.Context, botId int64, chatId string, text string, payType string) error {
	tokenText := ""
	if botId > 0 {
		if row, err := s.botById(ctx, botId); err == nil && row != nil {
			tokenText = row.BotToken
		}
	}
	if tokenText == "" {
		if row, err := s.officialBot(ctx); err == nil && row != nil {
			tokenText = row.BotToken
		}
	}
	if tokenText == "" {
		return nil
	}
	_, err := s.sendMessageWithMarkup(ctx, tokenText, chatId, text, "HTML", false, exchangeRateReplyKeyboard(payType))
	if err != nil && botId > 0 && shouldMarkBotOffline(err) {
		_ = s.markBotOffline(ctx, botId, err)
	}
	return err
}

func exchangeRateReplyKeyboard(payType string) *models.ReplyKeyboardMarkup {
	label := func(current string, text string) string {
		if payType == current {
			return "✅" + text
		}
		return text
	}
	return &models.ReplyKeyboardMarkup{Keyboard: [][]models.KeyboardButton{{
		{Text: label("all", "所有")},
		{Text: label("bank", "银行卡")},
		{Text: label("alipay", "支付宝")},
		{Text: label("wechat", "微信")},
	}}, IsPersistent: true, ResizeKeyboard: true, InputFieldPlaceholder: "实时汇率 1000*0.95/5"}
}
