package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
)

func TestCooperationApplicantWithoutUsername(t *testing.T) {
	applicant := &models.User{ID: 8379260686, FirstName: "测试", LastName: "申请人"}
	if got := cooperationApplicantDisplay(applicant); got != "测试 申请人" {
		t.Fatalf("cooperationApplicantDisplay() = %q", got)
	}
	button := cooperationApplicantButton(applicant)
	if button.Text != "测试 申请人" {
		t.Fatalf("cooperationApplicantButton().Text = %q", button.Text)
	}
	if button.URL != "tg://user?id=8379260686" {
		t.Fatalf("cooperationApplicantButton().URL = %q", button.URL)
	}
}

func TestCooperationApplicantWithUsername(t *testing.T) {
	applicant := &models.User{ID: 1, FirstName: "Pavel", LastName: "Durov", Username: "durov"}
	if got := cooperationApplicantDisplay(applicant); got != "@durov" {
		t.Fatalf("cooperationApplicantDisplay() = %q", got)
	}
	if got := cooperationApplicantButton(applicant).Text; got != "Pavel Durov" {
		t.Fatalf("cooperationApplicantButton().Text = %q", got)
	}
}

func TestCooperationChannelAdminRights(t *testing.T) {
	rights := cooperationChannelAdminRights()
	if !rights.ChangeInfo ||
		!rights.PostMessages ||
		!rights.EditMessages ||
		!rights.DeleteMessages ||
		!rights.PostStories ||
		!rights.EditStories ||
		!rights.DeleteStories ||
		!rights.InviteUsers {
		t.Fatalf("required channel admin rights are missing: %#v", rights)
	}
	if rights.Other || rights.BanUsers || rights.PinMessages || rights.AddAdmins || rights.ManageCall || rights.ManageTopics {
		t.Fatalf("unexpected channel admin rights enabled: %#v", rights)
	}
}

func TestCooperationApplicantResultText(t *testing.T) {
	application := &entity.YoubanTwoWayBotCooperationApplication{ErrorMessage: "频道A：管理员数量已满"}
	tests := map[string]string{
		sysin.CooperationJoinSuccess:       "合作申请已通过，机器人已加入采集频道。",
		sysin.CooperationJoinPartialFailed: "合作申请已通过，但部分频道添加失败：频道A：管理员数量已满 ⚠️ 请联系客服",
		sysin.CooperationJoinFailed:        "合作申请处理失败：频道A：管理员数量已满 ⚠️ 请联系客服",
	}
	for status, want := range tests {
		if got := cooperationApplicantResultText(application, status); got != want {
			t.Fatalf("cooperationApplicantResultText(%q) = %q, want %q", status, got, want)
		}
	}
	for _, status := range []string{sysin.CooperationReviewRejected, sysin.CooperationReviewCanceled, sysin.CooperationJoinRemoved} {
		if got := cooperationApplicantResultText(application, status); got != "" {
			t.Fatalf("cooperationApplicantResultText(%q) = %q, want empty", status, got)
		}
	}
}

func TestTwoWayBotMenusIncludeStart(t *testing.T) {
	feature := &twoWayChatGatewayFeature{}
	menus, err := feature.Menus(t.Context(), gatewayservice.BotContext{Bindings: []gatewayservice.BotBinding{{Owner: "youban_two_way_bot"}}})
	if err != nil {
		t.Fatal(err)
	}
	commands := map[string]bool{}
	for _, item := range menus.Items {
		commands[item.Command] = true
	}
	if !commands["start"] || !commands["chat"] {
		t.Fatalf("required commands missing: %#v", commands)
	}
}
