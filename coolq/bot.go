package coolq

import (
	"encoding/hex"
	"io"
	"os"
	"path"
	"runtime/debug"
	"time"

	"github.com/sam01101/MiraiGo-qdrive/client"
	"github.com/sam01101/MiraiGo-qdrive/message"
	"github.com/sam01101/MiraiGo-qdrive/utils"
	"github.com/sam01101/gocq-qqdrive/global"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// CQBot CQBot结构体,存储Bot实例相关配置
type CQBot struct {
	Client *client.QQClient

	events []func(MSG)
}

// MSG 消息Map
type MSG map[string]interface{}

// NewQQBot 初始化一个QQBot实例
func NewQQBot(cli *client.QQClient, conf *global.JSONConfig) *CQBot {
	bot := &CQBot{
		Client: cli,
	}
	go func() {
		i := conf.HeartbeatInterval
		if i < 0 {
			log.Warn("警告: 心跳功能已关闭，若非预期，请检查配置文件。")
			return
		}
		if i == 0 {
			i = 5
		}
		for {
			time.Sleep(time.Second * i)
			bot.dispatchEventMessage(MSG{
				"time":            time.Now().Unix(),
				"self_id":         bot.Client.Uin,
				"post_type":       "meta_event",
				"meta_event_type": "heartbeat",
				"interval":        1000 * i,
			})
		}
	}()
	return bot
}

// OnEventPush 注册事件上报函数
func (bot *CQBot) OnEventPush(f func(m MSG)) {
	bot.events = append(bot.events, f)
}

// UploadLocalVideo 上传本地短视频至群聊
func (bot *CQBot) UploadLocalVideo(target int64, v *LocalVideoElement) (*message.ShortVideoElement, error) {
	if v.File != "" {
		video, err := os.Open(v.File)
		if err != nil {
			return nil, err
		}
		defer video.Close()
		hash, _ := utils.ComputeMd5AndLength(io.MultiReader(video, v.thumb))
		cacheFile := path.Join(global.CachePath, hex.EncodeToString(hash[:])+".cache")
		_, _ = video.Seek(0, io.SeekStart)
		_, _ = v.thumb.Seek(0, io.SeekStart)
		return bot.Client.UploadGroupShortVideo(target, video, v.thumb, cacheFile)
	}
	return &v.ShortVideoElement, nil
}

func (bot *CQBot) dispatchEventMessage(m MSG) {
	for _, f := range bot.events {
		go func(fn func(MSG)) {
			defer func() {
				if pan := recover(); pan != nil {
					log.Warnf("处理事件 %v 时出现错误: %v \n%s", m, pan, debug.Stack())
				}
			}()
			start := time.Now()
			fn(m)
			end := time.Now()
			if end.Sub(start) > time.Second*5 {
				log.Debugf("警告: 事件处理耗时超过 5 秒 (%v), 请检查应用是否有堵塞.", end.Sub(start))
			}
		}(f)
	}
}

// ToJSON 生成JSON字符串
func (m MSG) ToJSON() string {
	b, _ := json.Marshal(m)
	return string(b)
}
