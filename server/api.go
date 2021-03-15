package server

import (
	"github.com/sam01101/gocq-qqdrive/coolq"
	"github.com/tidwall/gjson"
	"strings"
)

type resultGetter interface {
	Get(string) gjson.Result
}

type apiCaller struct {
	bot *coolq.CQBot
}

func getLoginInfo(bot *coolq.CQBot, _ resultGetter) coolq.MSG {
	return bot.CQGetLoginInfo()
}

func uploadShortVideo(bot *coolq.CQBot, p resultGetter) coolq.MSG {
	return bot.CQUploadShortVideo(p.Get("file").String())
}

func sendGroupForwardMSG(bot *coolq.CQBot, p resultGetter) coolq.MSG {
	return bot.CQSendGroupForwardMessage(p.Get("group_id").Int(), p.Get("messages"))
}

func getForwardMSG(bot *coolq.CQBot, p resultGetter) coolq.MSG {
	id := p.Get("message_id").Str
	if id == "" {
		id = p.Get("id").Str
	}
	return bot.CQGetForwardMessage(id)
}

func downloadFile(bot *coolq.CQBot, p resultGetter) coolq.MSG {
	headers := map[string]string{}
	headersToken := p.Get("headers")
	if headersToken.IsArray() {
		for _, sub := range headersToken.Array() {
			str := strings.SplitN(sub.String(), "=", 2)
			if len(str) == 2 {
				headers[str[0]] = str[1]
			}
		}
	}
	if headersToken.Type == gjson.String {
		lines := strings.Split(headersToken.String(), "\r\n")
		for _, sub := range lines {
			str := strings.SplitN(sub, "=", 2)
			if len(str) == 2 {
				headers[str[0]] = str[1]
			}
		}
	}
	return bot.CQDownloadFile(p.Get("url").Str, headers, int(p.Get("thread_count").Int()))
}

var API = map[string]func(*coolq.CQBot, resultGetter) coolq.MSG{
	"get_login_info":         getLoginInfo,
	"upload_short_video":     uploadShortVideo,
	"send_group_forward_msg": sendGroupForwardMSG,
	"get_forward_msg":        getForwardMSG,
	"download_file":          downloadFile,
}

func (api *apiCaller) callAPI(action string, p resultGetter) coolq.MSG {
	if f, ok := API[action]; ok {
		return f(api.bot, p)
	} else {
		return coolq.Failed(404, "API_NOT_FOUND", "API不存在")
	}
}
