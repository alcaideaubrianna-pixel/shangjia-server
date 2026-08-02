package sys

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"sort"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type telegramURLButton struct {
	Text string `json:"text"`
	URL  string `json:"url"`
	Row  int    `json:"row"`
}

func (s *sSysBot) bindSuccessMessage(ctx context.Context, app string, accountId int64, telegramUserId string, telegramUsername string) (string, models.ReplyMarkup) {
	values := map[string]interface{}{}
	if row, _ := s.featureConfig(ctx, (bindFeature{}).Key()); row != nil {
		values = featureConfigValues(row.ConfigJson)
	}
	text := fmt.Sprintf("%v", values["successText"])
	if strings.TrimSpace(text) == "" || text == "<nil>" {
		text = "<b>绑定成功</b>，回到页面即可查看绑定状态。"
	}
	accountName := ""
	if account, err := s.loginBoundAccount(ctx, app, accountId); err == nil && account != nil {
		accountName = firstNonEmpty(account.Nickname, account.Username)
	}
	tenantName := ""
	if app == "api" {
		var tenant struct {
			Name string `json:"name"`
		}
		_ = g.DB().Model(publishAccountTable+" a").Safe().Ctx(ctx).
			LeftJoin("hg_youban_publish_tenant t", "t.id=a.tenant_id").
			Fields("t.name").Where("a.id", accountId).Scan(&tenant)
		tenantName = tenant.Name
	}
	replacements := map[string]string{
		"{telegram_username}": html.EscapeString(telegramUsername),
		"{telegram_user_id}":  html.EscapeString(telegramUserId),
		"{account_name}":      html.EscapeString(accountName),
		"{tenant_name}":       html.EscapeString(tenantName),
	}
	for key, value := range replacements {
		text = strings.ReplaceAll(text, key, value)
	}
	buttons, _ := normalizeTelegramURLButtons(values["successButtons"])
	return sanitizeTelegramHTML(text), telegramURLButtonMarkup(buttons)
}

func normalizeTelegramURLButtons(value interface{}) ([]telegramURLButton, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, gerror.New("底部按钮配置不正确")
	}
	var buttons []telegramURLButton
	if string(data) == "null" || string(data) == `"<nil>"` {
		return buttons, nil
	}
	if err = json.Unmarshal(data, &buttons); err != nil {
		return nil, gerror.New("底部按钮配置不正确")
	}
	if len(buttons) > 8 {
		return nil, gerror.New("底部按钮最多配置8个")
	}
	result := make([]telegramURLButton, 0, len(buttons))
	for _, button := range buttons {
		button.Text = strings.TrimSpace(button.Text)
		button.URL = strings.TrimSpace(button.URL)
		if button.Text == "" || button.URL == "" {
			continue
		}
		parsed, parseErr := url.Parse(button.URL)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "tg") {
			return nil, gerror.Newf("按钮“%s”的链接不合法", button.Text)
		}
		if button.Row < 0 || button.Row > 7 {
			return nil, gerror.New("按钮行号必须在0到7之间")
		}
		result = append(result, button)
	}
	return result, nil
}

func telegramURLButtonMarkup(buttons []telegramURLButton) models.ReplyMarkup {
	if len(buttons) == 0 {
		return nil
	}
	sort.SliceStable(buttons, func(i, j int) bool { return buttons[i].Row < buttons[j].Row })
	rows := make([][]models.InlineKeyboardButton, 0)
	rowIndex := -1
	for _, button := range buttons {
		if button.Row != rowIndex {
			rows = append(rows, []models.InlineKeyboardButton{})
			rowIndex = button.Row
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], models.InlineKeyboardButton{Text: button.Text, URL: button.URL})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func sanitizeTelegramHTML(raw string) string {
	allowed := map[atom.Atom]string{
		atom.B: "b", atom.Strong: "b", atom.I: "i", atom.Em: "i", atom.U: "u",
		atom.S: "s", atom.Strike: "s", atom.Del: "s", atom.A: "a", atom.Code: "code",
		atom.Pre: "pre", atom.Blockquote: "blockquote", atom.Br: "br",
	}
	tokenizer := xhtml.NewTokenizer(strings.NewReader(raw))
	var output bytes.Buffer
	for {
		tokenType := tokenizer.Next()
		if tokenType == xhtml.ErrorToken {
			break
		}
		token := tokenizer.Token()
		tag, ok := allowed[token.DataAtom]
		switch tokenType {
		case xhtml.TextToken:
			output.WriteString(html.EscapeString(token.Data))
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			if !ok {
				continue
			}
			output.WriteByte('<')
			output.WriteString(tag)
			if tag == "a" {
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						if parsed, err := url.Parse(strings.TrimSpace(attr.Val)); err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https" || parsed.Scheme == "tg") {
							output.WriteString(` href="` + html.EscapeString(attr.Val) + `"`)
						}
					}
				}
			}
			output.WriteByte('>')
		case xhtml.EndTagToken:
			if ok && tag != "br" {
				output.WriteString("</" + tag + ">")
			}
		}
	}
	return strings.TrimSpace(output.String())
}
